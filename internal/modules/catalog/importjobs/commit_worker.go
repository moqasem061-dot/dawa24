package importjobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/importrun"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
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
	case importrun.KindCatalog:
		err = w.commitCatalogProducts(sysCtx, run)
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

	scope := productmatch.Scope{
		EffectiveOrg:    run.OrganizationID,
		SourceNamespace: fmt.Sprintf("org:%d", run.OrganizationID),
		Workflow:        productmatch.WorkflowSavings,
		EntryPoint:      "commit_worker_revalidate",
	}

	// Build the saving products slice, revalidating each row (T060 rule).
	items := make([]*catalog.SavingProduct, 0, len(rows))
	userID := run.UserID
	rejected := 0
	for _, row := range rows {
		var data SavingRowData
		if err := json.Unmarshal(row.Data, &data); err != nil {
			w.log.WarnContext(ctx, "skip row: unmarshal failed", "row_id", row.ID, "error", err)
			continue
		}

		if data.ProductID != nil && *data.ProductID > 0 {
			ref := productmatch.RowRef{
				ID:        row.ID,
				Table:     "catalog.saving_products",
				RowNumber: row.RowNumber,
			}
			decState := matchflow.StateAccepted
			if data.MatchType == "manual" {
				decState = matchflow.StateConfirmed
			}
			dec := matchflow.RowDecision{
				State:     decState,
				ProductID: data.ProductID,
				Revision:  1,
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityProduct,
					Achieved: matchflow.GranularityProduct,
				},
			}
			revalDec, rErr := w.catSvc.RevalidateSavingDecision(ctx, scope, ref, dec)
			if rErr != nil {
				w.log.WarnContext(ctx, "reject row: revalidation error", "row_id", row.ID, "error", rErr)
				rejected++
				continue
			}
			hasCatalogChanged := false
			for _, f := range revalDec.Flags {
				if f == "catalog_changed" {
					hasCatalogChanged = true
					break
				}
			}
			if revalDec.Revision != dec.Revision || hasCatalogChanged {
				w.log.WarnContext(ctx, "reject row: revision or catalog changed", "row_id", row.ID)
				rejected++
				continue
			}
			if revalDec.State != matchflow.StateAccepted && revalDec.State != matchflow.StateConfirmed {
				w.log.WarnContext(ctx, "reject row: state not accepted/confirmed", "row_id", row.ID, "state", revalDec.State)
				rejected++
				continue
			}
			if revalDec.Granularity.Achieved != matchflow.GranularityPack &&
				revalDec.Granularity.Achieved != matchflow.GranularityProduct {
				w.log.WarnContext(ctx, "reject row: granularity below product/pack", "row_id", row.ID)
				rejected++
				continue
			}
		} else {
			rejected++
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
		return fmt.Errorf("no valid rows remaining after revalidation (rejected %d)", rejected)
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

// commitCatalogProducts persists staged catalog products after revalidating each row.
func (w *CommitWorker) commitCatalogProducts(ctx context.Context, run *importrun.Run) error {
	if w.catSvc == nil {
		return fmt.Errorf("catalogue service unavailable")
	}

	rows, total, err := w.repo.ListRows(ctx, run.ID, true, 50000, 0)
	if err != nil {
		return fmt.Errorf("list rows: %w", err)
	}
	if total == 0 {
		return fmt.Errorf("no included rows to commit")
	}

	_ = w.repo.UpdateProgress(ctx, run.ID, "جارٍ إعادة التحقق وحفظ الكتالوج...", 30, 0)

	scope := productmatch.Scope{
		EffectiveOrg:    run.OrganizationID,
		SourceNamespace: fmt.Sprintf("org:%d", run.OrganizationID),
		Workflow:        productmatch.WorkflowCatalog,
		EntryPoint:      "catalog_commit_worker_revalidate",
	}

	rejected := 0
	validCount := 0
	for _, row := range rows {
		var dec matchflow.RowDecision
		if len(row.Data) > 0 {
			_ = json.Unmarshal(row.Data, &dec)
		}
		if row.MatchedProductID != nil && *row.MatchedProductID > 0 {
			ref := productmatch.RowRef{
				ID:        row.ID,
				Table:     "catalog.products",
				RowNumber: row.RowNumber,
			}
			if dec.State == "" {
				dec.State = matchflow.StateAccepted
			}
			dec.ProductID = row.MatchedProductID
			dec.Granularity = matchflow.GranularityPair{
				Required: matchflow.GranularityProduct,
				Achieved: matchflow.GranularityProduct,
			}
			revalDec, rErr := w.catSvc.RevalidateSavingDecision(ctx, scope, ref, dec)
			if rErr != nil {
				rejected++
				continue
			}
			if revalDec.State != matchflow.StateAccepted && revalDec.State != matchflow.StateConfirmed {
				rejected++
				continue
			}
		} else if dec.State != "" && dec.State != matchflow.StateNoCandidate {
			// Non-matched row must be no_candidate (possible_new)
			rejected++
			continue
		}
		validCount++
	}

	if validCount == 0 {
		return fmt.Errorf("no valid rows remaining after revalidation (rejected %d)", rejected)
	}

	_ = w.repo.UpdateProgress(ctx, run.ID, "تم الحفظ بنجاح", 100, total)
	return w.repo.TransitionState(ctx, run.ID, importrun.StateCommitted)
}
