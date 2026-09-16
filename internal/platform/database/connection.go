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

	"github.com/muhiya/dawa24-store/internal/platform/config"
	_ "github.com/muhiya/dawa24-store/internal/shared/timeutil"
)

// ErrNotConnected is returned when the pool has not been established yet.
var ErrNotConnected = errors.New("database: not connected yet")

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

	db.logRLSBypassNotice(ctx, pool)
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
	db.logRLSBypassNotice(ctx, pool)
	return db, nil
}

func (db *DB) logRLSBypassNotice(ctx context.Context, pool *pgxpool.Pool) {
	bypassed := roleBypassesRLS(ctx, pool)
	db.rlsBypassed.Store(bypassed)
	if bypassed {
		slog.Warn("SECURITY NOTICE: Database connection role has SUPERUSER or BYPASSRLS privileges; engine-level Row-Level Security is INERT. Connect via non-superuser role (e.g. dawa24_app) in production.",
			"role_bypasses_rls", true)
	}
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
	poolCfg.ConnConfig.RuntimeParams["application_name"] = "dawa24-store"
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
