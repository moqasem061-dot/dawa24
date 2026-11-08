package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
)

// NewCleanupWorker builds the nightly ERP retention worker (the erp.cleanup
// periodic job).
func NewCleanupWorker(svc *erp.Service, log *slog.Logger) *CleanupWorker {
	return &CleanupWorker{svc: svc, log: log}
}

// CleanupWorker delegates to the service's retention sweep; the sweep itself
// bounds every statement by CleanupBatchRows.
type CleanupWorker struct {
	river.WorkerDefaults[queue.ERPCleanupArgs]
	svc *erp.Service
	log *slog.Logger
}

func (w *CleanupWorker) Work(ctx context.Context, job *river.Job[queue.ERPCleanupArgs]) error {
	start := time.Now()
	if err := w.svc.Cleanup(ctx, time.Now()); err != nil {
		if w.log != nil {
			w.log.ErrorContext(ctx, "erp cleanup failed", "error", err)
		}
		return err
	}
	if w.log != nil {
		w.log.InfoContext(ctx, "erp cleanup done", "took", time.Since(start).Round(time.Millisecond))
	}
	return nil
}
