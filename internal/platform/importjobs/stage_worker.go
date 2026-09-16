// Package importjobs implements River workers for the unified import
// queue: stage (parse + match) and commit (persist to target tables).
//
// Each worker type is registered in cmd/worker/main.go and processes
// jobs from the "imports" queue.
package importjobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/platform/queue"
)

// ──────────────────────────────────────────────────────────────────────
// Stage worker: parse a file, match rows against the catalogue, store
// the staged results in platform.import_run_rows.
// ──────────────────────────────────────────────────────────────────────

// StageWorker processes ImportStageArgs jobs.
type StageWorker struct {
	river.WorkerDefaults[queue.ImportStageArgs]
	db     *database.DB
	repo   importrun.Repository
	catSvc *catalog.Service
	log    *slog.Logger
}

// NewStageWorker creates a StageWorker.
func NewStageWorker(db *database.DB, repo importrun.Repository, catSvc *catalog.Service, log *slog.Logger) *StageWorker {
	return &StageWorker{db: db, repo: repo, catSvc: catSvc, log: log}
}

// Work processes a single import stage job.
func (w *StageWorker) Work(ctx context.Context, job *river.Job[queue.ImportStageArgs]) error {
	runID := job.Args.RunID
	w.log.InfoContext(ctx, "import stage job started", "run_id", runID, "org_id", job.Args.OrganizationID)

	sysCtx := database.AsSystem(ctx)
	run, err := w.repo.GetRunByID(sysCtx, runID)
	if err != nil {
		w.log.ErrorContext(ctx, "import stage: run not found", "run_id", runID, "error", err)
		return err
	}

	// Guard: only process runs that are still queued.
	if run.State != importrun.StateQueued {
		w.log.WarnContext(ctx, "import stage: run not in queued state, skipping",
			"run_id", runID, "state", run.State)
		return nil
	}

	// Transition to processing.
	if err := w.repo.TransitionState(sysCtx, runID, importrun.StateProcessing); err != nil {
		return fmt.Errorf("import stage: transition to processing: %w", err)
	}

	// Dispatch to the appropriate kind handler.
	switch run.Kind {
	case importrun.KindSavingProducts:
		err = w.stageSavingProducts(sysCtx, run)
	default:
		err = fmt.Errorf("import stage: unsupported kind %q", run.Kind)
	}

	if err != nil {
		w.log.ErrorContext(ctx, "import stage failed", "run_id", runID, "kind", run.Kind, "error", err)
		if failErr := w.repo.FailRun(sysCtx, runID, err.Error()); failErr != nil {
			w.log.ErrorContext(ctx, "import stage: failed to mark run as failed", "run_id", runID, "error", failErr)
		}
		// Return nil so River does not retry (MaxAttempts: 1).
		return nil
	}

	w.log.InfoContext(ctx, "import stage job completed", "run_id", runID, "kind", run.Kind)
	return nil
}
