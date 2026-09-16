package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// VendorWarehouseClearStocksSubmit deletes all stock records for the given warehouse.
func (h *UIHandler) VendorWarehouseClearStocksSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	whID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || whID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", i18n.T(lang, "common.invalid_id"))
		return
	}

	// Verify warehouse ownership
	wh, err := h.invSvc.GetWarehouse(ctx, whID)
	if err != nil || wh == nil {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", i18n.T(lang, "admin.warehouses.not_found"))
		return
	}
	if wh.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", i18n.T(lang, "common.unauthorized"))
		return
	}

	if err := h.invSvc.ClearWarehouseStocks(ctx, whID); err != nil {
		h.log.ErrorContext(ctx, "failed to clear warehouse stocks", "warehouse_id", whID, "error", err)
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/warehouses/%d", whID), "error", "تعذر تصفير المخزون: "+err.Error())
		return
	}

	h.log.InfoContext(ctx, "cleared all warehouse stocks", "warehouse_id", whID, "org_id", actor.OrgID)
	h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/warehouses/%d", whID), "success", "تم تصفير وحذف كافة أرصدة المخزون في هذا المخزن بنجاح.")
}
