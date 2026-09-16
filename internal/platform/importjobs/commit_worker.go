package importjobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/platform/queue"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ──────────────────────────────────────────────────────────────────────
// Commit worker: persist the staged + reviewed rows into their
// destination tables.
// ──────────────────────────────────────────────────────────────────────

// CommitWorker processes ImportCommitArgs jobs.
type CommitWorker struct {
	river.WorkerDefaults[queue.ImportCommitArgs]
	db     *database.DB
	repo   importrun.Repository
	catSvc *catalog.Service
	log    *slog.Logger
}

// NewCommitWorker creates a CommitWorker.
func NewCommitWorker(db *database.DB, repo importrun.Repository, catSvc *catalog.Service, log *slog.Logger) *CommitWorker {
	return &CommitWorker{db: db, repo: repo, catSvc: catSvc, log: log}
}

// Work processes a single import commit job.
func (w *CommitWorker) Work(ctx context.Context, job *river.Job[queue.ImportCommitArgs]) error {
	runID := job.Args.RunID
	w.log.InfoContext(ctx, "import commit job started", "run_id", runID)

	sysCtx := database.AsSystem(ctx)
	run, err := w.repo.GetRunByID(sysCtx, runID)
	if err != nil {
		return fmt.Errorf("import commit: run not found: %w", err)
	}

	// Guard: only commit runs that are ready.
	if run.State != importrun.StateReady {
		w.log.WarnContext(ctx, "import commit: run not ready, skipping",
			"run_id", runID, "state", run.State)
		return nil
	}

	// Transition to committing.
	if err := w.repo.TransitionState(sysCtx, runID, importrun.StateCommitting); err != nil {
		return fmt.Errorf("import commit: transition: %w", err)
	}

	_ = w.repo.UpdateProgress(sysCtx, runID, "جارٍ حفظ البيانات...", 10, 0)

	switch run.Kind {
	case importrun.KindSavingProducts:
		err = w.commitSavingProducts(sysCtx, run)
	default:
		err = fmt.Errorf("import commit: unsupported kind %q", run.Kind)
	}

	if err != nil {
		w.log.ErrorContext(ctx, "import commit failed", "run_id", runID, "error", err)
		if failErr := w.repo.FailRun(sysCtx, runID, err.Error()); failErr != nil {
			w.log.ErrorContext(ctx, "import commit: failed to mark as failed", "error", failErr)
		}
		return err // Allow River to retry (MaxAttempts: 3).
	}

	w.log.InfoContext(ctx, "import commit job completed", "run_id", runID)
	return nil
}

// commitSavingProducts persists saving product rows into catalog.saving_products.
func (w *CommitWorker) commitSavingProducts(ctx context.Context, run *importrun.Run) error {
	if w.catSvc == nil {
		return fmt.Errorf("catalogue service unavailable")
	}

	// Read all included rows.
	rows, total, err := w.repo.ListRows(ctx, run.ID, true, 50000, 0)
	if err != nil {
		return fmt.Errorf("list rows: %w", err)
	}
	if total == 0 {
		return fmt.Errorf("no included rows to commit")
	}

	_ = w.repo.UpdateProgress(ctx, run.ID, "جارٍ حفظ البيانات...", 30, 0)

	// Build the saving products slice.
	items := make([]*catalog.SavingProduct, 0, len(rows))
	userID := run.UserID
	for _, row := range rows {
		var data SavingRowData
		if err := json.Unmarshal(row.Data, &data); err != nil {
			w.log.WarnContext(ctx, "skip row: unmarshal failed", "row_id", row.ID, "error", err)
			continue
		}

		items = append(items, &catalog.SavingProduct{
			OrganizationID: run.OrganizationID,
			UserID:         &userID,
			ProductID:      data.ProductID,
			NameProduct:    data.NameProduct,
			SKU:            data.SKU,
			Quantity:       data.Quantity,
			Price:          money.FromMinor(data.PriceMinor),
		})
	}

	if len(items) == 0 {
		return fmt.Errorf("no valid rows after parsing")
	}

	_ = w.repo.UpdateProgress(ctx, run.ID, "جارٍ حفظ البيانات...", 60, 0)

	// Batch upsert into saving_products.
	added, updated, err := w.catSvc.BatchUpsertSavingProducts(ctx, run.OrganizationID, &userID, items)
	if err != nil {
		return fmt.Errorf("batch upsert: %w", err)
	}

	// Update result with commit counts.
	var existingResult SavingResult
	if run.Result != nil {
		_ = json.Unmarshal(run.Result, &existingResult)
	}
	existingResult.InsertedCount = added
	existingResult.UpdatedCount = updated
	resultJSON, _ := json.Marshal(existingResult)
	if err := w.repo.SetResult(ctx, run.ID, resultJSON); err != nil {
		w.log.ErrorContext(ctx, "set commit result", "error", err)
	}

	_ = w.repo.UpdateProgress(ctx, run.ID, "تم الحفظ بنجاح", 100, total)
	return w.repo.TransitionState(ctx, run.ID, importrun.StateCommitted)
}
