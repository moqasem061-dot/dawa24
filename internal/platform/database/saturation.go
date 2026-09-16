package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Telling a saturated pool apart from a client that went away.
//
// Both arrive as `context canceled`, and the difference is everything. One
// means the browser navigated off and nothing is wrong; the other means every
// connection is in use and every request is now queuing behind the same wall.
// Reported identically, the second is invisible until someone notices the whole
// site is slow — which is exactly how it presented: a bare "context canceled"
// with no indication that the database was the thing at fault.
//
// pgx offers no error for "the pool was full", because from its point of view
// nothing failed: it waited, as asked, until the caller stopped waiting. So the
// distinction is drawn from the pool's own counters at the moment of failure,
// which are exact and cost nothing to read.

// ErrPoolExhausted means every connection was in use and none came free before
// the caller gave up. It is a capacity problem, not a query problem.
var ErrPoolExhausted = errors.New("database: connection pool exhausted")

// ErrServerConnectionsExhausted means PostgreSQL itself refused a new
// connection. No amount of pool tuning fixes this one: something is holding
// connections server-side, and max_connections has been reached.
var ErrServerConnectionsExhausted = errors.New("database: server refused a new connection; max_connections reached")

// diagnose turns a failure to begin a transaction into an error that names the
// cause.
//
// It is called only on the error path, so the Stat() read never touches a
// healthy request.
func diagnose(ctx context.Context, pool *pgxpool.Pool, err error) error {
	if err == nil {
		return nil
	}

	// PostgreSQL said no outright: 53300 is too_many_connections. That is the
	// server's limit rather than ours, and it is worth naming separately
	// because the remedy is different — find what is holding connections, or
	// raise max_connections.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "53300" {
		return fmt.Errorf("%w: %v", ErrServerConnectionsExhausted, err)
	}

	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("database: begin: %w", err)
	}

	// A context error with a full pool is a queue, not a cancellation. The
	// caller may genuinely have gone away at the same moment, but a full pool
	// is the more useful thing to report and the one an operator can act on.
	stat := pool.Stat()
	if stat.AcquiredConns() >= stat.MaxConns() {
		return fmt.Errorf("%w (%d/%d in use, %d waiting): %v",
			ErrPoolExhausted, stat.AcquiredConns(), stat.MaxConns(),
			stat.EmptyAcquireCount(), err)
	}

	// The pool had room, so the caller really did go away. Nothing is wrong.
	return fmt.Errorf("database: begin: %w", err)
}

// PoolStats is what /health reports about connection capacity.
type PoolStats struct {
	Acquired int32 `json:"acquired"`
	Idle     int32 `json:"idle"`
	Max      int32 `json:"max"`
	// Waiting is the cumulative count of acquisitions that had to wait for a
	// connection. It only ever rises, so a jump between two health checks is
	// the signal, not the absolute number.
	Waiting int64 `json:"waiting"`
	// Saturated means every connection is currently in use. The next request
	// will queue, and if it queues past its own deadline the caller sees a
	// cancellation with no indication that capacity was the reason.
	Saturated bool `json:"saturated"`
}

// Stats reports connection capacity.
//
// It exists so saturation is visible on the health endpoint rather than only
// inferable from a scattering of cancelled requests. A pool at its ceiling is
// the single most common cause of a site that is "slow for no reason".
func (db *DB) Stats() PoolStats {
	db.mu.RLock()
	pool := db.pool
	db.mu.RUnlock()
	if pool == nil {
		return PoolStats{}
	}
	s := pool.Stat()
	return PoolStats{
		Acquired:  s.AcquiredConns(),
		Idle:      s.IdleConns(),
		Max:       s.MaxConns(),
		Waiting:   s.EmptyAcquireCount(),
		Saturated: s.AcquiredConns() >= s.MaxConns(),
	}
}
