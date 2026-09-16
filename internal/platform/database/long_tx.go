package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Letting one transaction take longer than the pool's ceiling allows.
//
// statement_timeout is set as a RuntimeParam on every connection in the pool,
// and it is set there deliberately: it is the difference between one
// pathological query and a saturated pool, and the legacy system had neither.
// Thirty seconds is right for a web request.
//
// It is not right for a background job. The web process runs the import stage
// and commit workers inline in a goroutine — see cmd/server/imports.go — so a
// saving-products import writing twenty-five thousand staged rows was doing it
// against a thirty-second ceiling, whatever River's thirty-MINUTE JobTimeout
// said. What the operator saw was not a job timing out; it was one statement
// cancelled by Postgres, surfacing as an import that failed with a message
// about a statement timeout naming nothing they could act on.
//
// cmd/worker now opens its pool with its own ceiling. This is the other half:
// the same relief for the same work when it runs in the web process, granted
// one transaction at a time rather than by raising the ceiling for everything.

// LongStatementTimeout is what a background transaction is allowed.
//
// Ten minutes, matching DB_WORKER_STATEMENT_TIMEOUT's default so a job behaves
// the same in either process. It is still a ceiling: a runaway query in a
// background transaction lets go of its connection rather than holding one of
// twenty for ever.
const LongStatementTimeout = 10 * time.Minute

// InLongTx runs fn inside a read-write transaction whose statements may take up
// to LongStatementTimeout.
//
// Use it for work that is genuinely long and genuinely in the background: bulk
// staging writes, a commit that touches every row of an import. Never for
// anything on a request path — a web request that needs ten minutes has a
// problem this would only hide.
//
// SET LOCAL scopes the change to this transaction, so the connection returns to
// the pool with the ordinary ceiling restored. A session-level SET would leak
// ten minutes of patience into the next request that borrowed the connection,
// which is the same trap applyTenant exists to avoid for the tenant id.
func (db *DB) InLongTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if err := setLocalStatementTimeout(txCtx, tx, LongStatementTimeout); err != nil {
			return err
		}
		return fn(txCtx, tx)
	})
}

// setLocalStatementTimeout raises the ceiling for the current transaction.
//
// The value is interpolated rather than parameterised because SET LOCAL does
// not accept a bind parameter, and set_config's third argument is the local
// flag. It is a duration this package controls, never user input.
func setLocalStatementTimeout(ctx context.Context, tx pgx.Tx, d time.Duration) error {
	ms := d.Milliseconds()
	if ms <= 0 {
		return nil
	}
	_, err := tx.Exec(ctx, "SELECT set_config('statement_timeout', $1, true)", fmt.Sprintf("%d", ms))
	if err != nil {
		return fmt.Errorf("database: raise statement timeout: %w", err)
	}
	return nil
}
