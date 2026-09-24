package ui

import (
	"context"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/modules/notifications"
)

// notifySmartOrderRunFinished alerts the user when a smart-ordering calculation finishes.
func (h *UIHandler) notifySmartOrderRunFinished(ctx context.Context, userID int64, orgID int64, runID int64) {
	vars := map[string]string{
		"run_id": fmt.Sprintf("%d", runID),
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchEvent(ctx, notifications.EventSmartOrderRunFinished, userID, orgPtr, vars)
}

// notifySmartOrderRunFailed alerts the user when a smart-ordering run fails.
func (h *UIHandler) notifySmartOrderRunFailed(ctx context.Context, userID int64, orgID int64, runID int64, errMsg string) {
	vars := map[string]string{
		"run_id": fmt.Sprintf("%d", runID),
		"error":  errMsg,
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchEvent(ctx, notifications.EventSmartOrderRunFailed, userID, orgPtr, vars)
}

// notifyImportRunFinished alerts the user when an import finishes committing.
func (h *UIHandler) notifyImportRunFinished(ctx context.Context, userID int64, orgID int64, runID int64, rowCount int) {
	vars := map[string]string{
		"run_id":     fmt.Sprintf("%d", runID),
		"rows_count": fmt.Sprintf("%d", rowCount),
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchEvent(ctx, notifications.EventImportRunFinished, userID, orgPtr, vars)
}

// notifyImportRunFailed alerts the user when an import run fails.
func (h *UIHandler) notifyImportRunFailed(ctx context.Context, userID int64, orgID int64, runID int64, errMsg string) {
	vars := map[string]string{
		"run_id": fmt.Sprintf("%d", runID),
		"error":  errMsg,
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchEvent(ctx, notifications.EventImportRunFailed, userID, orgPtr, vars)
}

// notifyQuotaExhausted alerts the buying organization when a branch quota is reached.
func (h *UIHandler) notifyQuotaExhausted(ctx context.Context, buyingOrgID int64, branchName, vendorName, productName string) {
	if buyingOrgID <= 0 {
		return
	}
	vars := map[string]string{
		"branch_name":  branchName,
		"vendor_name":  vendorName,
		"product_name": productName,
	}
	h.dispatchOrgEvent(ctx, notifications.EventQuotaExhausted, buyingOrgID, vars)
}

// notifyQuotaReleased alerts the buying organization when a supplier releases quota.
func (h *UIHandler) notifyQuotaReleased(ctx context.Context, buyingOrgID int64, branchName, vendorName, productName string) {
	if buyingOrgID <= 0 {
		return
	}
	vars := map[string]string{
		"branch_name":  branchName,
		"vendor_name":  vendorName,
		"product_name": productName,
	}
	h.dispatchOrgEvent(ctx, notifications.EventQuotaReleased, buyingOrgID, vars)
}
