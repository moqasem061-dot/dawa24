package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// VendorTransferCancelSubmit cancels an in-transit warehouse transfer and restores stock to source.
func (h *UIHandler) VendorTransferCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/transfers", http.StatusSeeOther)
		return
	}

	transferID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if h.invSvc != nil && transferID > 0 {
		if _, err := h.invSvc.CancelTransfer(ctx, transferID, "إلغاء يدوي من سجل الحركات"); err != nil {
			h.redirectWithNotice(w, r, "/vendor/transfers", "error", "فشل إلغاء التحويل: "+h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/vendor/transfers", "success", "تم إلغاء التحويل واستعادة الرصيد للمخزن المصدر بنجاح.")
}

// VendorStockAdjustSubmit adjusts a stock level with a reason.
func (h *UIHandler) VendorStockAdjustSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/inventory", http.StatusSeeOther)
		return
	}

	redirectURL := r.PostFormValue("redirect_url")
	if redirectURL == "" {
		redirectURL = "/vendor/inventory"
	}

	stockID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	delta, _ := strconv.Atoi(r.PostFormValue("delta"))
	if h.invSvc != nil && stockID > 0 && delta != 0 {
		if _, err := h.invSvc.AdjustStock(ctx, inventory.AdjustStockInput{
			StockID: stockID,
			Delta:   delta,
			Type:    inventory.MovementAdjustment,
			Details: r.PostFormValue("reason"),
			UserID:  &actor.UserID,
		}); err != nil {
			h.logLoadErr(ctx, "invSvc.AdjustStock", err)
		}
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// recordInitialStock writes a variant's opening quantity into inventory.stocks.
//
// inventory.stocks.warehouse_id is NOT NULL, so this needs somewhere to put it.
// A supplier with no warehouse yet gets a clear message rather than a number
// that disappears.
func (h *UIHandler) recordInitialStock(ctx context.Context, orgID int64, v *catalog.ProductVariant, qty int) error {
	if h.invSvc == nil {
		return fmt.Errorf("inventory service unavailable")
	}
	warehouses, err := h.invSvc.ListWarehouses(ctx)
	if err != nil {
		return err
	}
	var warehouseID int64
	// Match warehouse associated with the variant's branch
	if v.BranchID != nil && *v.BranchID > 0 {
		for _, wh := range warehouses {
			if wh.OrganizationID == orgID && wh.BranchID != nil && *wh.BranchID == *v.BranchID && wh.IsActive {
				warehouseID = wh.ID
				break
			}
		}
	} else {
		// Fallback to any active warehouse of the vendor only if no branch is specified
		for _, wh := range warehouses {
			if wh.OrganizationID == orgID && wh.IsActive {
				warehouseID = wh.ID
				break
			}
		}
	}

	// If no warehouse exists, auto-create a real warehouse linked to the vendor's branch
	if warehouseID == 0 {
		whName := i18n.T("ar", "vendor.ingest.main_warehouse")
		whCode := "WH-MAIN"
		if v.BranchID != nil && *v.BranchID > 0 {
			whCode = fmt.Sprintf("WH-BR-%d", *v.BranchID)
			if h.orgSvc != nil {
				if b, err := h.orgSvc.GetBranch(ctx, *v.BranchID); err == nil && b != nil {
					whName = i18n.T("ar", "vendor.inventory.warehouse_prefix") + b.Name.Get(i18n.AR)
				}
			}
		}
		newWh := &inventory.Warehouse{
			OrganizationID: orgID,
			BranchID:       v.BranchID,
			Name:           whName,
			Code:           whCode,
			IsActive:       true,
		}
		createdWh, err := h.invSvc.CreateWarehouse(ctx, newWh)
		if err == nil && createdWh != nil {
			warehouseID = createdWh.ID
		}
	}
	if warehouseID == 0 {
		return fmt.Errorf("organization %d has no warehouse to hold stock", orgID)
	}
	return h.invSvc.SetStock(ctx, &inventory.Stock{
		OrganizationID:   orgID,
		WarehouseID:      warehouseID,
		ProductID:        v.ProductID,
		ProductVariantID: v.ID,
		Quantity:         qty,
	})
}
