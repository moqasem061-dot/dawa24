package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	_ "github.com/moqasem061-dot/dawa24/internal/shared/timeutil"
)

// ErrNotConnected is returned when the pool has not been established yet.
var ErrNotConnected = errors.New("database: not connected yet")

// ErrRLSBypassed is returned when row-level security is required but the
// connecting role is a superuser or holds BYPASSRLS. It is a configuration
// error, not a transient one: retrying cannot fix it.
var ErrRLSBypassed = errors.New("database: the connecting role bypasses row-level security; " +
	"connect the server and worker as dawa24_app (APP_DATABASE_URL), not as the owner or a superuser")

// DB wraps the pool and exposes only transaction-scoped access.
//
// The handle is created before the pool exists and is filled in once dialling
// succeeds. That indirection matters: the HTTP server starts before its
// dependencies are up (see cmd/server/deps.go), so routes are mounted — and
// repositories constructed — while the database is still connecting. Handing
// those repositories a *DB that is nil at that moment would leave every one of
// them holding a nil pointer forever, which is exactly the panic this replaced.
type DB struct {
	mu   sync.RWMutex
	pool *pgxpool.Pool
	// rlsBypassed records that the connecting role is a superuser or holds
	// BYPASSRLS. Row-level security then never applies, so the tenant GUC a
	// transaction sets for it changes nothing and its round trip is skipped.
	rlsBypassed atomic.Bool
}

// New returns an unconnected handle. Call Connect to establish the pool.
func New() *DB { return &DB{} }

// Connect dials PostgreSQL and attaches the pool to this handle.
//
// Safe to call repeatedly: a successful connection replaces any previous pool
// and closes it, so a retry loop cannot leak connections.
func (db *DB) Connect(ctx context.Context, cfg config.Database) error {
	pool, err := newPool(ctx, cfg)
	if err != nil {
		return err
	}
	if err := db.checkRLS(ctx, pool, cfg.RequireRLS); err != nil {
		pool.Close()
		return err
	}

	db.mu.Lock()
	old := db.pool
	db.pool = pool
	db.mu.Unlock()

	if old != nil {
		old.Close()
	}
	return nil
}

// Connected reports whether the pool is established.
func (db *DB) Connected() bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.pool != nil
}

func (db *DB) getPool() (*pgxpool.Pool, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.pool == nil {
		return nil, ErrNotConnected
	}
	return db.pool, nil
}

// Open builds the pool and verifies connectivity. A process that cannot reach
// its database should fail at boot, not on its first request.
func Open(ctx context.Context, cfg config.Database) (*DB, error) {
	pool, err := newPool(ctx, cfg)
	if err != nil {
		return nil, err
	}
	db := &DB{pool: pool}
	if err := db.checkRLS(ctx, pool, cfg.RequireRLS); err != nil {
		pool.Close()
		return nil, err
	}
	return db, nil
}

// checkRLS records whether the connecting role bypasses row-level security,
// and refuses it when required. A superuser connection makes every RLS policy
// inert, so tenant isolation then rests on each query's WHERE clause alone.
func (db *DB) checkRLS(ctx context.Context, pool *pgxpool.Pool, required bool) error {
	bypassed := roleBypassesRLS(ctx, pool)
	db.rlsBypassed.Store(bypassed)
	owns := !bypassed && roleOwnsAppTables(ctx, pool)
	if !bypassed && !owns {
		return nil
	}
	if required {
		return ErrRLSBypassed
	}
	slog.Error("SECURITY NOTICE: the database role is a superuser, holds BYPASSRLS, or owns the application tables; row-level security is inert or can be switched off. Connect the server and worker as dawa24_app.",
		"role_bypasses_rls", bypassed, "role_owns_tables", owns)
	return nil
}

func newPool(ctx context.Context, cfg config.Database) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("database: parse DATABASE_URL: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.HealthCheckPeriod = time.Minute

	// A statement timeout is the difference between one pathological query and a
	// saturated pool. The legacy system had neither, which is why a single
	// unindexed admin report could stall checkout.
	if poolCfg.ConnConfig.RuntimeParams == nil {
		poolCfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	poolCfg.ConnConfig.RuntimeParams["statement_timeout"] =
		fmt.Sprintf("%d", cfg.StatementTimeout.Milliseconds())
	poolCfg.ConnConfig.RuntimeParams["application_name"] = "dawa24"
	poolCfg.ConnConfig.RuntimeParams["timezone"] = "Africa/Cairo"
	poolCfg.ConnConfig.Tracer = statsTracer{}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}

	return pool, nil
}

// Close releases the pool if one is attached.
func (db *DB) Close() {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.pool != nil {
		db.pool.Close()
		db.pool = nil
	}
}

// Health verifies the database answers, for the /health endpoint and the
// container healthcheck.
func (db *DB) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := db.getPool()
	if err != nil {
		return err
	}

	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("database: health: %w", err)
	}
	return nil
}

// Pool exposes the raw pool for the River queue driver, which manages its own
// transactions. Application code must not use this.
func (db *DB) Pool() *pgxpool.Pool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.pool
}

// roleBypassesRLS reports whether the pool's role is exempt from row-level
// security. An error reports false, which keeps the tenant GUC being set: the
// safe direction.
//
// Every transaction used to send set_config for the tenant before its first
// statement. Under a superuser that statement arms nothing, and on a database
// reached over the network it was one extra round trip on nearly every read
// the platform makes. The policy helpers (platform.current_org_id,
// platform.is_system, platform.tenant_visible) are the only readers of those
// settings; no trigger or default reads them.
func roleBypassesRLS(ctx context.Context, pool *pgxpool.Pool) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var bypass bool
	err := pool.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&bypass)
	return err == nil && bypass
}

// roleOwnsAppTables reports whether the connecting role owns application
// tables. FORCE ROW LEVEL SECURITY binds an owner only until it runs ALTER
// TABLE ... DISABLE ROW LEVEL SECURITY, which an owner may, and an owner can
// drop the tables outright; a runtime role must own nothing. public is
// excluded because the job queue may create its tables there.
func roleOwnsAppTables(ctx context.Context, pool *pgxpool.Pool) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var owns bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM pg_class c
			  JOIN pg_namespace n ON n.oid = c.relnamespace
			 WHERE c.relowner = (SELECT oid FROM pg_roles WHERE rolname = current_user)
			   AND c.relkind IN ('r', 'p')
			   AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'public')
			   AND n.nspname NOT LIKE 'pg\_%')`).Scan(&owns)
	return err == nil && owns
}
