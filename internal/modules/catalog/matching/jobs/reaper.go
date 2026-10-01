package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
)

// DefaultStaleHeartbeatAge is the duration after which an unresponsive match run is marked interrupted (45s per US12).
const DefaultStaleHeartbeatAge = 45 * time.Second

// MatchReaperWorker sweeps match runs stuck in 'preparing' or 'enhancing' whose heartbeat is stale.
type MatchReaperWorker struct {
	river.WorkerDefaults[queue.MatchReaperArgs]
	db  *database.DB
	log *slog.Logger
}

// NewMatchReaperWorker constructs a MatchReaperWorker.
func NewMatchReaperWorker(db *database.DB, log *slog.Logger) *MatchReaperWorker {
	return &MatchReaperWorker{
		db:  db,
		log: log,
	}
}

// Work runs the periodic reaper sweep across all tenants using database.AsSystem.
func (w *MatchReaperWorker) Work(ctx context.Context, job *river.Job[queue.MatchReaperArgs]) error {
	count, err := SweepStaleRuns(ctx, w.db, DefaultStaleHeartbeatAge)
	if err != nil {
		w.log.ErrorContext(ctx, "match reaper sweep failed", "error", err)
		return err
	}
	if count > 0 {
		w.log.WarnContext(ctx, "reaped stale match runs", "count", count)
	}
	return nil
}

// SweepStaleRuns updates all preparing or enhancing match runs with a stale heartbeat to interrupted.
// It runs under database.AsSystem as it is a cross-tenant background maintenance sweep.
func SweepStaleRuns(ctx context.Context, db *database.DB, maxAge time.Duration) (int64, error) {
	if db == nil {
		return 0, nil
	}
	if maxAge <= 0 {
		maxAge = DefaultStaleHeartbeatAge
	}

	var affected int64
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.match_runs
			SET state = 'interrupted',
			    finished_at = now()
			WHERE state IN ('preparing', 'enhancing')
			  AND heartbeat_at < now() - $1::interval;
		`
		tag, execErr := tx.Exec(txCtx, query, fmt.Sprintf("%d seconds", int(maxAge.Seconds())))
		if execErr != nil {
			return fmt.Errorf("reap stale match runs: %w", execErr)
		}
		affected = tag.RowsAffected()
		return nil
	})
	return affected, err
}
