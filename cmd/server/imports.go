package main

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/importjobs"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	importrunPostgres "github.com/muhiya/dawa24-store/internal/platform/importrun/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/progress"
	"github.com/muhiya/dawa24-store/internal/platform/queue"
	"github.com/muhiya/dawa24-store/internal/platform/safe"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// wireImports configures durable import runs and background workers for the UI.
//
// Like wireSmartOrder, it wires the Stage and Commit workers so imports can
// execute asynchronously with database-backed progress. If cmd/worker is
// deployed, River workers claim jobs from the "imports" queue. When running
// as a standalone server, the dispatch closures run the workers in a detached
// goroutine so imports never hang in 'queued'.
func wireImports(
	db *database.DB,
	uiHandler *ui.UIHandler,
	catSvc *catalog.Service,
	hub *progress.Hub,
	publisher *progress.Publisher,
	log *slog.Logger,
) {
	if db == nil || uiHandler == nil {
		return
	}

	// The hub and publisher are built by the caller, because the import STORES
	// are decorated with the same publisher and they are wired before this runs.
	// One hub per process, or two screens watching the same run would subscribe
	// to different fan-outs and only one of them would be fed.
	uiHandler.SetProgressHub(hub)

	repo := importrun.WithProgress(importrunPostgres.New(db), publisher)
	uiHandler.SetImportRunRepo(repo)

	stageWorker := importjobs.NewStageWorker(db, repo, catSvc, log)
	commitWorker := importjobs.NewCommitWorker(db, repo, catSvc, log)

	stageFn := func(ctx context.Context, runID, orgID int64) error {
		safe.Go(log, "server-import-stage", func() {
			bgCtx := context.Background()
			job := &river.Job[queue.ImportStageArgs]{
				Args: queue.ImportStageArgs{
					RunID:          runID,
					OrganizationID: orgID,
				},
			}
			if err := stageWorker.Work(bgCtx, job); err != nil {
				log.Error("import stage execution failed", "run_id", runID, "error", err)
			}
		})
		return nil
	}

	commitFn := func(ctx context.Context, runID, orgID int64) error {
		safe.Go(log, "server-import-commit", func() {
			bgCtx := context.Background()
			job := &river.Job[queue.ImportCommitArgs]{
				Args: queue.ImportCommitArgs{
					RunID:          runID,
					OrganizationID: orgID,
				},
			}
			if err := commitWorker.Work(bgCtx, job); err != nil {
				log.Error("import commit execution failed", "run_id", runID, "error", err)
			}
		})
		return nil
	}

	uiHandler.SetImportQueue(stageFn, commitFn)
}
