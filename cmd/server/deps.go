package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/platform/cache"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// dependencies holds connections that may not be available at boot.
//
// The server deliberately starts BEFORE these are connected. A process that
// exits because PostgreSQL was briefly unreachable leaves the reverse proxy
// with nothing to talk to, and the operator sees a bare "502 Bad Gateway" with
// no indication of what failed. Starting anyway means /health answers, /ready
// explains precisely which dependency is down, and the platform can distinguish
// "still starting" from "misconfigured".
//
// Configuration errors are different and still abort at boot: a bad DATABASE_URL
// is a deployment mistake that no amount of retrying will fix.
type dependencies struct {
	ctx      context.Context
	mu       sync.RWMutex
	db       *database.DB
	cache    *cache.Cache
	dbErr    error
	cacheErr error
	ready    bool
	// capsule lets the Telegram bridge reach the assistant mounted inside the
	// authenticated API group. See telegram_wiring.go.
	capsule *capsuleBridge
	// actions lets the assistant act through the UI handler built after it.
	actions *lateActions
}

func newDependencies(ctx context.Context) *dependencies {
	return &dependencies{
		ctx:      ctx,
		// The handle exists from the start and gains its pool when dialling
		// succeeds. Routes are mounted before the database is up, so
		// repositories must be handed a pointer that becomes usable later
		// rather than one that is nil at mount time and nil forever after.
		db:       database.New(),
		cache:    nil, // set in connect once the environment is known
		dbErr:    errNotConnectedYet,
		cacheErr: errNotConnectedYet,
	}
}

type notConnected struct{}

func (notConnected) Error() string { return "not connected yet" }

var errNotConnectedYet = notConnected{}

func (d *dependencies) Context() context.Context {
	if d != nil && d.ctx != nil {
		return d.ctx
	}
	return context.Background()
}

func (d *dependencies) DB() (*database.DB, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.db, d.dbErr
}

func (d *dependencies) Cache() (*cache.Cache, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cache, d.cacheErr
}

// Ready reports whether every dependency connected at least once.
func (d *dependencies) Ready() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ready
}

// setDBErr records connection state. The handle itself never changes.
func (d *dependencies) setDBErr(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dbErr = err
	d.recompute()
}

// Handle returns the database handle, which is non-nil from construction even
// before the pool is dialled. Callers that need to know whether it is usable
// yet ask DB() instead.
func (d *dependencies) Handle() *database.DB { return d.db }

// setCacheErr records connection state. The handle itself never changes.
func (d *dependencies) setCacheErr(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cacheErr = err
	d.recompute()
}

// recompute must be called with the write lock held.
func (d *dependencies) recompute() {
	d.ready = d.dbErr == nil && d.cacheErr == nil
}

func (d *dependencies) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db != nil {
		d.db.Close()
	}
	if d.cache != nil {
		_ = d.cache.Close()
	}
}

// connect dials both dependencies in the background, retrying until the context
// is cancelled.
//
// Retrying forever rather than giving up after N attempts is deliberate: on a
// fresh deploy the database service may genuinely still be provisioning, and a
// container that exits after 30 seconds turns a slow start into a crash loop
// that hides the original cause.
// CacheHandle returns the cache handle, non-nil from the first connect call.
func (d *dependencies) CacheHandle() *cache.Cache {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cache
}

func (d *dependencies) connect(ctx context.Context, cfg *config.Config, log *slog.Logger) {
	// Create the handle before dialling so anything constructed during route
	// mounting holds a pointer that becomes usable later.
	d.mu.Lock()
	d.cache = cache.New(cfg.Env)
	d.mu.Unlock()

	go retryForever(ctx, log, "database", func(ctx context.Context) error {
		if err := d.db.Connect(ctx, cfg.Database); err != nil {
			d.setDBErr(err)
			return err
		}
		// Ensure migrations are applied as soon as the database is reachable
		migrations, migLoadErr := database.LoadMigrations(dbfs.Migrations, "migrations")
		if migLoadErr == nil {
			if migErr := d.db.Migrate(ctx, migrations, func(msg string, args ...any) {
				log.Info(msg, args...)
			}); migErr != nil {
				log.Warn("background migration execution note", "error", migErr)
			}
		}
		// The permission catalogue is defined in Go and mirrored into
		// identity.permissions. Syncing here, on the connection that just came
		// up, means a deploy that adds a permission has it grantable before the
		// first request rather than after somebody notices the checkbox missing.
		//
		// A failure is logged, not fatal: the mirror only feeds the role
		// editor, and enforcement reads the catalogue from Go directly. A
		// process that refuses to serve because a role editor would be stale
		// turns a cosmetic problem into an outage.
		if err := rbac.Sync(ctx, d.db); err != nil {
			log.Error("could not sync the permission catalogue", "error", err)
		} else if seeded, err := rbac.SeedExistingCompanies(ctx, d.db); err != nil {
			log.Error("could not seed company roles", "error", err)
		} else {
			if seeded > 0 {
				log.Info("seeded starter roles for organizations", "organizations", seeded)
			}
			// A role declared after a company was seeded — مندوب توصيل is the
			// first — reaches that company here and nowhere else.
			if added, err := rbac.SeedMissingSystemRoles(ctx, d.db); err != nil {
				log.Error("could not seed newly declared starter roles", "error", err)
			} else if added > 0 {
				log.Info("seeded newly declared starter roles", "organizations", added)
			}
			if repaired, err := rbac.RepairOutOfScopeGrants(ctx, d.db); err != nil {
				log.Error("could not repair company role grants", "error", err)
			} else if repaired > 0 {
				log.Info("repaired company role grants after a scope change",
					"organizations", repaired)
			}
		}
		d.setDBErr(nil)
		return nil
	})

	go retryForever(ctx, log, "redis", func(ctx context.Context) error {
		if err := d.CacheHandle().Connect(ctx, cfg.Redis); err != nil {
			d.setCacheErr(err)
			return err
		}
		d.setCacheErr(nil)
		return nil
	})
}

func retryForever(ctx context.Context, log *slog.Logger, name string, dial func(context.Context) error) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return
		}

		err := dial(ctx)
		if err == nil {
			log.Info("dependency connected", "dependency", name, "attempts", attempt)
			return
		}

		// Logged at warn, not error: during a normal cold start this is
		// expected for a few seconds and should not page anyone.
		log.Warn("dependency unavailable, retrying",
			"dependency", name,
			"attempt", attempt,
			"retry_in", backoff.String(),
			"error", err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}
