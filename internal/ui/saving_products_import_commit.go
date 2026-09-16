package ui

import (
	"context"
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
)

// Committing a staged saving-products import.
//
// Split out of saving_products_import_run_ops.go, which owns starting a run,
// because the two answer different questions and the file had grown past the
// 400-line limit AGENTS.md sets.

// commitSavingImportRun commits staged saving items, attempting the in-memory
// session first and falling back to platform.import_run_rows if the session expired
// or the process restarted.
func (h *UIHandler) commitSavingImportRun(
	ctx context.Context,
	sessionID string,
	orgID, userID int64,
	catSvc *catalog.Service,
) (added int, updated int, err error) {
	// 1. Try in-memory session first.
	added, updated, err = globalSavingImportSessionStore.CommitSession(ctx, sessionID, orgID, userID, catSvc)
	if err == nil {
		if h.importRunRepo != nil {
			if run, rErr := h.importRunRepo.GetRunByPublicID(ctx, sessionID, orgID); rErr == nil && run != nil {
				_ = h.importRunRepo.TransitionState(ctx, run.ID, importrun.StateCommitted)
			}
		}
		return added, updated, nil
	}

	// 2. Transitional fallback to database rows if in-memory session was lost.
	if h.importRunRepo != nil {
		run, rErr := h.importRunRepo.GetRunByPublicID(ctx, sessionID, orgID)
		if rErr == nil && run != nil {
			rows, total, lErr := h.importRunRepo.ListRows(ctx, run.ID, true, 50000, 0)
			if lErr == nil && total > 0 {
				itemsToCommit := make([]*catalog.SavingProduct, 0, len(rows))
				for _, r := range rows {
					var item StagedSavingItem
					if jErr := json.Unmarshal(r.Data, &item); jErr == nil && item.Included {
						itemsToCommit = append(itemsToCommit, &catalog.SavingProduct{
							OrganizationID: orgID,
							UserID:         &userID,
							ProductID:      item.ProductID,
							NameProduct:    item.NameProduct,
							SKU:            item.SKU,
							Quantity:       item.Quantity,
							Price:          item.Price,
						})
					}
				}
				if len(itemsToCommit) > 0 && catSvc != nil {
					a, u, cErr := catSvc.BatchUpsertSavingProducts(ctx, orgID, &userID, itemsToCommit)
					if cErr == nil {
						_ = h.importRunRepo.TransitionState(ctx, run.ID, importrun.StateCommitted)
						return a, u, nil
					}
					return 0, 0, cErr
				}
			}
		}
	}

	return 0, 0, err
}
