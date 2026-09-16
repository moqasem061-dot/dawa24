// Package jobs runs a smart order in the background.
//
// Processing is a job rather than a request because a ten-thousand-row import is
// minutes of work: holding an HTTP connection open for it would fail on the
// first proxy timeout, and would strand the run if the buyer closed the tab.
// Running it here is also what makes the run resumable — the buyer can come back
// from another device and find it where it got to (FR-027, US8).
package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder/pipeline"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/queue"
)

// BranchResolver supplies the delivery branch's coordinates.
//
// An interface because branches belong to org, and modules must not import each
// other.
type BranchResolver interface {
	Location(ctx context.Context, orgID, branchID int64) (lat, lng float64, ok bool, err error)
}

// RunWorker executes a smart ordering run.
type RunWorker struct {
	river.WorkerDefaults[queue.SmartOrderRunArgs]
	repo     smartorder.Repository
	runner   *pipeline.Runner
	branches BranchResolver
	log      *slog.Logger
}

// NewRunWorker constructs the worker.
func NewRunWorker(repo smartorder.Repository, runner *pipeline.Runner,
	branches BranchResolver, log *slog.Logger) *RunWorker {
	return &RunWorker{repo: repo, runner: runner, branches: branches, log: log}
}

// Work executes the pipeline for one run.
//
// A failure marks the run failed with a reason the buyer can read, rather than
// leaving it in `processing` forever. River's retry then gets another attempt;
// the pipeline is written so a second pass over the same rows produces the same
// result rather than duplicating work.
func (w *RunWorker) Work(ctx context.Context, job *river.Job[queue.SmartOrderRunArgs]) error {
	runID := job.Args.RunID
	orgID := job.Args.OrganizationID

	ctx = database.WithTenant(ctx, orgID)
	ctx = authctx.ContextWithActor(ctx, authctx.Actor{
		OrganizationID: orgID,
		OrgID:          orgID,
		Role:           "customer",
	})

	run, err := w.repo.GetRunByID(ctx, orgID, runID)
	if err != nil {
		return fmt.Errorf("load run %d: %w", runID, err)
	}

	if run.Status == smartorder.StatusPlaced {
		// Someone finalised while this sat in the queue. Re-running would
		// rewrite the lines behind a placed order.
		w.log.InfoContext(ctx, "skipping smart order run that is already placed", "run_id", runID)
		return nil
	}
	if run.Status != smartorder.StatusQueued {
		// River may redeliver a job after a transport failure. Never restart a
		// run that another worker already owns or that has already produced
		// results; retrying it would repeat the AI stage.
		w.log.InfoContext(ctx, "skipping smart order run that is not queued",
			"run_id", runID, "status", run.Status)
		return nil
	}

	cfg, err := w.repo.GetConfig(ctx, runID)
	if err != nil {
		return w.fail(ctx, run, fmt.Errorf("load configuration: %w", err))
	}

	lat, lng, hasCoord, err := w.branches.Location(ctx, orgID, run.BranchID)
	if err != nil {
		return w.fail(ctx, run, fmt.Errorf("resolve branch: %w", err))
	}
	branch := pipeline.BranchLocation{
		BranchID: run.BranchID, Lat: lat, Lng: lng, HasCoord: hasCoord,
	}
	if !hasCoord {
		// Worth saying out loud: without coordinates every coverage check passes,
		// so the buyer may be offered suppliers who cannot reach them.
		w.log.WarnContext(ctx, "branch has no coordinates; coverage cannot be enforced",
			"run_id", runID, "branch_id", run.BranchID)
	}

	if claimer, ok := w.repo.(smartorder.RunClaimer); ok {
		claimed, err := claimer.ClaimRun(ctx, orgID, runID)
		if err != nil {
			return err
		}
		if !claimed {
			w.log.InfoContext(ctx, "smart order run claim lost", "run_id", runID)
			return nil
		}
		run.Status = smartorder.StatusProcessing
	} else {
		if err := run.TransitionTo(smartorder.StatusProcessing); err != nil {
			return w.fail(ctx, run, err)
		}
		if err := w.repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, ""); err != nil {
			return err
		}
	}

	if err := w.runner.Execute(ctx, run, cfg, branch); err != nil {
		return w.fail(ctx, run, err)
	}

	if err := run.TransitionTo(smartorder.StatusCompleted); err != nil {
		return w.fail(ctx, run, err)
	}
	return w.repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, "")
}

// fail records the reason on the run and returns the error to River.
func (w *RunWorker) fail(ctx context.Context, run *smartorder.Run, cause error) error {
	w.log.ErrorContext(ctx, "smart order run failed", "run_id", run.ID, "error", cause)
	_ = w.repo.UpdateRunStatus(ctx, run.ID, smartorder.StatusFailed, run.CurrentStep, cause.Error())
	return cause
}
