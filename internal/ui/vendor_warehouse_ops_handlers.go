package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// VendorWarehouseStockAdjustSubmit adjusts a stock level directly from warehouse detail page.
func (h *UIHandler) VendorWarehouseStockAdjustSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	whID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	stockID, _ := strconv.ParseInt(chi.URLParam(r, "stockID"), 10, 64)
	if whID <= 0 || stockID <= 0 {
		http.Redirect(w, r, "/vendor/warehouses", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/warehouses/%d", whID), "error", i18n.T(langOf(r), "common.form_invalid"))
		return
	}

	reason := strings.TrimSpace(r.PostFormValue("reason"))
	if reason == "" {
		reason = i18n.T(langOf(r), "vendor.warehouse.default_adjustment_reason")
	}

	if h.invSvc != nil {
		if newQtyStr := strings.TrimSpace(r.PostFormValue("new_quantity")); newQtyStr != "" {
			if newQty, err := strconv.Atoi(newQtyStr); err == nil && newQty >= 0 {
				stocks, _ := h.invSvc.ListStocksByWarehouse(ctx, whID)
				var currentStock *inventory.Stock
				for _, st := range stocks {
					if st.ID == stockID {
						currentStock = st
						break
					}
				}
				if currentStock != nil {
					delta := newQty - currentStock.Quantity
					if delta != 0 {
						_, err := h.invSvc.AdjustStock(ctx, inventory.AdjustStockInput{
							StockID: stockID,
							Delta:   delta,
							Type:    inventory.MovementAdjustment,
							Details: reason,
							UserID:  &actor.UserID,
						})
						if err != nil {
							h.log.ErrorContext(ctx, "adjust stock failed", "error", err)
							h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/warehouses/%d", whID), "error", fmt.Sprintf(i18n.T(langOf(r), "vendor.warehouse.adjust_error"), h.safeMessage(err, langOf(r))))
							return
						}
					}
				}
			}
		} else if deltaStr := strings.TrimSpace(r.PostFormValue("delta")); deltaStr != "" {
			if delta, err := strconv.Atoi(deltaStr); err == nil && delta != 0 {
				_, err := h.invSvc.AdjustStock(ctx, inventory.AdjustStockInput{
					StockID: stockID,
					Delta:   delta,
					Type:    inventory.MovementAdjustment,
					Details: reason,
					UserID:  &actor.UserID,
				})
				if err != nil {
					h.log.ErrorContext(ctx, "adjust stock failed", "error", err)
					h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/warehouses/%d", whID), "error", fmt.Sprintf(i18n.T(langOf(r), "vendor.warehouse.adjust_error"), h.safeMessage(err, langOf(r))))
					return
				}
			}
		}
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/warehouses/%d", whID), "success", i18n.T(langOf(r), "vendor.warehouse.adjust_success"))
}

// VendorWarehouseCreateSubmit handles creating a new warehouse facility for the vendor.
func (h *UIHandler) VendorWarehouseCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", i18n.T(langOf(r), "common.form_invalid"))
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	code := strings.TrimSpace(r.PostFormValue("code"))
	address := strings.TrimSpace(r.PostFormValue("address"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))

	if name == "" {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", i18n.T(langOf(r), "vendor.warehouse.name_required"))
		return
	}
	if code == "" {
		code = fmt.Sprintf("WH-%d-%d", actor.OrganizationID, 100+int(time.Now().UnixNano()%900))
	}

	var branchID *int64
	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}

	var lat, lng *float64
	if latStr := r.PostFormValue("latitude"); latStr != "" {
		if v, err := strconv.ParseFloat(latStr, 64); err == nil {
			lat = &v
		}
	}
	if lngStr := r.PostFormValue("longitude"); lngStr != "" {
		if v, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lng = &v
		}
	}

	// Auto-fallback from branch coordinates if omitted
	if (lat == nil || lng == nil) && branchID != nil && h.orgSvc != nil {
		if b, err := h.orgSvc.GetBranch(ctx, *branchID); err == nil && b != nil {
			if lat == nil {
				lat = b.Latitude
			}
			if lng == nil {
				lng = b.Longitude
			}
			if address == "" && b.Address != "" {
				address = b.Address
			}
			if phone == "" && b.Phone != "" {
				phone = b.Phone
			}
		}
	}

	wh := &inventory.Warehouse{
		OrganizationID: actor.OrganizationID,
		BranchID:       branchID,
		Name:           name,
		Code:           code,
		Address:        address,
		Phone:          phone,
		Latitude:       lat,
		Longitude:      lng,
		IsActive:       true,
	}

	if h.invSvc != nil {
		if _, err := h.invSvc.CreateWarehouse(ctx, wh); err != nil {
			h.log.ErrorContext(ctx, "create warehouse failed", "error", err)
			h.redirectWithNotice(w, r, "/vendor/warehouses", "error", i18n.T(langOf(r), "vendor.warehouse.create_error"))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/warehouses", "success", i18n.T(langOf(r), "vendor.warehouse.create_success"))
}

// VendorWarehouseUpdateSubmit handles editing an existing warehouse facility.
func (h *UIHandler) VendorWarehouseUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	idStr := chi.URLParam(r, "id")
	whID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || whID <= 0 {
		http.Redirect(w, r, "/vendor/warehouses", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", i18n.T(langOf(r), "common.form_invalid"))
		return
	}

	var wh *inventory.Warehouse
	if h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(ctx)
		for _, item := range whs {
			if item.ID == whID && item.OrganizationID == actor.OrganizationID {
				wh = item
				break
			}
		}
	}

	if wh == nil {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", i18n.T(langOf(r), "vendor.warehouse.not_found"))
		return
	}

	wh.Name = strings.TrimSpace(r.PostFormValue("name"))
	if wh.Name == "" {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", i18n.T(langOf(r), "vendor.warehouse.name_required"))
		return
	}
	if code := strings.TrimSpace(r.PostFormValue("code")); code != "" {
		wh.Code = code
	}
	wh.Address = strings.TrimSpace(r.PostFormValue("address"))
	wh.Phone = strings.TrimSpace(r.PostFormValue("phone"))
	wh.IsActive = r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on"

	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			wh.BranchID = &bID
		}
	}

	if h.invSvc != nil {
		if _, err := h.invSvc.UpdateWarehouse(ctx, wh.ID, wh); err != nil {
			h.log.ErrorContext(ctx, "update warehouse failed", "error", err)
			h.redirectWithNotice(w, r, "/vendor/warehouses", "error", i18n.T(langOf(r), "vendor.warehouse.update_error"))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/warehouses", "success", i18n.T(langOf(r), "vendor.warehouse.update_success"))
}

// VendorWarehouseToggleSubmit toggles the active/inactive status of a warehouse.
func (h *UIHandler) VendorWarehouseToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	idStr := chi.URLParam(r, "id")
	whID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || whID <= 0 {
		http.Redirect(w, r, "/vendor/warehouses", http.StatusSeeOther)
		return
	}

	var wh *inventory.Warehouse
	if h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(ctx)
		for _, item := range whs {
			if item.ID == whID && item.OrganizationID == actor.OrganizationID {
				wh = item
				break
			}
		}
	}

	if wh == nil {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", i18n.T(langOf(r), "vendor.warehouse.not_found"))
		return
	}

	wh.IsActive = !wh.IsActive
	if h.invSvc != nil {
		ctx = database.WithTenant(ctx, actor.OrganizationID)
		_, _ = h.invSvc.UpdateWarehouse(ctx, wh.ID, wh)
	}

	msg := i18n.T(langOf(r), "vendor.warehouse.deactivated")
	if wh.IsActive {
		msg = i18n.T(langOf(r), "vendor.warehouse.activated")
	}
	h.redirectWithNotice(w, r, "/vendor/warehouses", "success", msg)
}
