package ui

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/importrun"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Committing a staged saving-products import.
//
// Split out of saving_products_import_run_ops.go, which owns starting a run,
// because the two answer different questions and the file had grown past the
// 400-line limit AGENTS.md sets.

// RevalidateSavingItems checks each staged item before commit (matching the rule in T060).
// Rows whose revision changed, whose state is not accepted/confirmed, or whose
// granularity is below pack/product are rejected.
func RevalidateSavingItems(
	ctx context.Context,
	orgID, userID int64,
	items []*StagedSavingItem,
	catSvc *catalog.Service,
) (valid []*catalog.SavingProduct, rejected int, err error) {
	if len(items) == 0 {
		return nil, 0, nil
	}

	scope := productmatch.Scope{
		EffectiveOrg:    orgID,
		SourceNamespace: fmt.Sprintf("org:%d", orgID),
		Workflow:        productmatch.WorkflowSavings,
		EntryPoint:      "savings_commit_revalidate",
	}

	valid = make([]*catalog.SavingProduct, 0, len(items))
	for _, it := range items {
		if it == nil || !it.Included {
			continue
		}

		if it.ProductID != nil && *it.ProductID > 0 {
			ref := productmatch.RowRef{
				ID:        int64(it.Index),
				Table:     "catalog.saving_products",
				RowNumber: it.Index + 1,
			}

			decState := matchflow.StateAccepted
			if it.MatchType == "manual" {
				decState = matchflow.StateConfirmed
			}

			dec := matchflow.RowDecision{
				State:     decState,
				ProductID: it.ProductID,
				Revision:  1,
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityProduct,
					Achieved: matchflow.GranularityProduct,
				},
			}

			if catSvc != nil {
				revalDec, rErr := catSvc.RevalidateSavingDecision(ctx, scope, ref, dec)
				if rErr != nil {
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
					rejected++
					continue
				}

				if revalDec.State != matchflow.StateAccepted && revalDec.State != matchflow.StateConfirmed {
					rejected++
					continue
				}

				if revalDec.Granularity.Achieved != matchflow.GranularityPack &&
					revalDec.Granularity.Achieved != matchflow.GranularityProduct {
					rejected++
					continue
				}
			}
		}

		valid = append(valid, &catalog.SavingProduct{
			OrganizationID: orgID,
			UserID:         &userID,
			ProductID:      it.ProductID,
			NameProduct:    it.NameProduct,
			SKU:            it.SKU,
			Quantity:       it.Quantity,
			Price:          it.Price,
		})
	}

	return valid, rejected, nil
}

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
	if err == nil { // nolint:errswallow: err is returned to the caller; this only records success
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
				stagedSnapshot := make([]*StagedSavingItem, 0, len(rows))
				for _, r := range rows {
					var item StagedSavingItem
					if jErr := json.Unmarshal(r.Data, &item); jErr == nil && item.Included {
						stagedSnapshot = append(stagedSnapshot, &item)
					}
				}
				itemsToCommit, _, revalErr := RevalidateSavingItems(ctx, orgID, userID, stagedSnapshot, catSvc)
				if revalErr == nil && len(itemsToCommit) > 0 && catSvc != nil {
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
