// Package jobs holds the River workers the erpsync queue runs. They are thin:
// scope the context to the args' organization and call the service — all the
// behavior lives in the service and its repository.
package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/erpgate"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
)

// NewSyncChunkWorker builds the erpsync chunk worker.
func NewSyncChunkWorker(svc *erp.Service, log *slog.Logger) *SyncChunkWorker {
	return &SyncChunkWorker{svc: svc, log: log}
}

// SyncChunkWorker processes one chunk of one accepted ERP batch.
type SyncChunkWorker struct {
	river.WorkerDefaults[queue.ERPSyncChunkArgs]
	svc *erp.Service
	log *slog.Logger
}

// disabledSnooze is how long a chunk waits before the worker looks at the ERP
// switch again while the integration is turned off.
const disabledSnooze = 5 * time.Minute

// Work re-scopes the context to the batch's organization (the job args carry
// it; the queue is shared, the data is not) and calls ProcessChunk. An
// earlier-batch wait becomes a snooze of the SyncSnoozeSeconds constant —
// never a literal, never a failure (research D-09); every other error passes
// through to River's MaxAttempts retry.
func (w *SyncChunkWorker) Work(ctx context.Context, job *river.Job[queue.ERPSyncChunkArgs]) error {
	// While an admin has the integration switched off, a chunk is put back, not
	// run and not discarded: switching it on again resumes the batch where it
	// stopped.
	if !erpgate.Enabled(ctx) {
		return river.JobSnooze(disabledSnooze)
	}
	err := w.svc.ProcessChunk(database.WithTenant(ctx, job.Args.OrganizationID), job.Args.BatchID, job.Args.ChunkNo)
	if errors.Is(err, erp.ErrWaitForEarlierBatch) {
		return river.JobSnooze(time.Duration(erp.SyncSnoozeSeconds) * time.Second)
	}
	if err != nil && w.log != nil {
		w.log.ErrorContext(ctx, "erp sync chunk failed", "batch", job.Args.BatchID, "chunk", job.Args.ChunkNo, "error", err)
	}
	return err
}
