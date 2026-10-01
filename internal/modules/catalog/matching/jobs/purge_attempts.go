package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
)

// DefaultAttemptRetentionAge is the maximum age of match_ai_attempts rows before purging (30 days per US9 T129).
const DefaultAttemptRetentionAge = 30 * 24 * time.Hour

// PurgeAttemptsWorker periodically deletes match_ai_attempts older than 30 days.
type PurgeAttemptsWorker struct {
	river.WorkerDefaults[queue.MatchPurgeAttemptsArgs]
	repo matching.Repository
	log  *slog.Logger
}

// NewPurgeAttemptsWorker constructs a PurgeAttemptsWorker.
func NewPurgeAttemptsWorker(repo matching.Repository, log *slog.Logger) *PurgeAttemptsWorker {
	return &PurgeAttemptsWorker{
		repo: repo,
		log:  log,
	}
}

// Work runs the periodic purge of obsolete AI attempts.
func (w *PurgeAttemptsWorker) Work(ctx context.Context, job *river.Job[queue.MatchPurgeAttemptsArgs]) error {
	count, err := w.repo.PurgeOldAIAttempts(ctx, DefaultAttemptRetentionAge)
	if err != nil {
		w.log.ErrorContext(ctx, "purge AI attempts failed", "error", err)
		return err
	}
	if count > 0 {
		w.log.InfoContext(ctx, "purged old AI attempts", "count", count)
	}
	return nil
}
