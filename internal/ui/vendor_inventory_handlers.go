package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorStockAlertsPage renders the dedicated stock alerts view with low-stock products, warehouses, and branches.
func (h *UIHandler) VendorStockAlertsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/inventory/alerts", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	whID, _ := strconv.ParseInt(r.URL.Query().Get("warehouse_id"), 10, 64)
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))

	var pagedStocks []*inventory.Stock
	var allLowStocks []*inventory.Stock
	var total int
	var warehouses []*inventory.Warehouse
	var branches []*org.Branch

	if h.invSvc != nil {
		allWhs, _ := h.invSvc.ListWarehouses(ctx)
		for _, wh := range allWhs {
			if wh.OrganizationID == actor.OrganizationID {
				warehouses = append(warehouses, wh)
			}
		}

		allOrgStocks, _ := h.invSvc.ListStocksByOrg(ctx, actor.OrganizationID)
		for _, s := range allOrgStocks {
			if s.Quantity <= s.MinThreshold {
				allLowStocks = append(allLowStocks, s)
			}
		}

		if statusFilter == "out_of_stock" || statusFilter == "low_stock" {
			var matched []*inventory.Stock
			for _, s := range allLowStocks {
				if whID > 0 && s.WarehouseID != whID {
					continue
				}
				if statusFilter == "out_of_stock" && s.Quantity > 0 {
					continue
				}
				if statusFilter == "low_stock" && s.Quantity <= 0 {
					continue
				}
				matched = append(matched, s)
			}
			total = len(matched)
			start := offset
			if start > total {
				start = total
			}
			end := start + limit
			if end > total {
				end = total
			}
			pagedStocks = matched[start:end]
		} else {
			pagedStocks, total, _ = h.invSvc.ListLowStocksByOrgWithTotal(ctx, actor.OrganizationID, whID, q, limit, offset)
		}
	}

	if h.orgSvc != nil {
		branches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	}

	var variants []*catalog.ProductVariant
	if h.catSvc != nil && len(pagedStocks) > 0 {
		var productIDs []int64
		for _, s := range pagedStocks {
			if s != nil {
				productIDs = append(productIDs, s.ProductID)
			}
		}
		if len(productIDs) > 0 {
			vMap, _ := h.catSvc.ListVariantsByProducts(ctx, productIDs)
			for _, vList := range vMap {
				variants = append(variants, vList...)
			}
		}
	}

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	data := pages.VendorStockAlertsData{
		Stocks:       pagedStocks,
		AllLowStocks: allLowStocks,
		Warehouses:   warehouses,
		Branches:     branches,
		Variants:     variants,
		Total:        total,
		Page:         page,
		PerPage:      limit,
		TotalPages:   totalPages,
		Query:        q,
		WarehouseID:  whID,
		StatusFilter: statusFilter,
		NoticeType:   r.URL.Query().Get("notice_type"),
		NoticeMsg:    r.URL.Query().Get("notice"),
	}

	h.renderPage(ctx, w, "render vendor stock alerts page", pages.VendorStockAlerts(data, lang, dir, h.isHTMX(r)))
}

// VendorInventoryPage renders the inventory stock view with search, warehouse filtering, and pagination.
func (h *UIHandler) VendorInventoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/inventory", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	whID, _ := strconv.ParseInt(r.URL.Query().Get("warehouse_id"), 10, 64)

	var pagedStocks []*inventory.Stock
	var allStocks []*inventory.Stock
	var total int
	var warehouses []*inventory.Warehouse
	if h.invSvc != nil {
		pagedStocks, total, _ = h.invSvc.ListStocksByOrgWithTotal(ctx, actor.OrganizationID, whID, q, limit, offset)
		allStocks, _ = h.invSvc.ListStocksByOrg(ctx, actor.OrganizationID)
		allWhs, _ := h.invSvc.ListWarehouses(ctx)
		for _, wh := range allWhs {
			if wh.OrganizationID == actor.OrganizationID {
				warehouses = append(warehouses, wh)
			}
		}
	}

	var variants []*catalog.ProductVariant
	if h.catSvc != nil && len(pagedStocks) > 0 {
		var productIDs []int64
		for _, s := range pagedStocks {
			if s != nil {
				productIDs = append(productIDs, s.ProductID)
			}
		}
		if len(productIDs) > 0 {
			vMap, _ := h.catSvc.ListVariantsByProducts(ctx, productIDs)
			for _, vList := range vMap {
				variants = append(variants, vList...)
			}
		}
	}

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	data := pages.VendorInventoryData{
		Stocks:      pagedStocks,
		AllStocks:   allStocks,
		Warehouses:  warehouses,
		Variants:    variants,
		Total:       total,
		Page:        page,
		PerPage:     limit,
		TotalPages:  totalPages,
		Query:       q,
		WarehouseID: whID,
		NoticeType:  r.URL.Query().Get("notice_type"),
		NoticeMsg:   r.URL.Query().Get("notice"),
	}

	h.renderPage(ctx, w, "render vendor inventory page", pages.VendorInventory(data, lang, dir, h.isHTMX(r)))
}

// VendorTransfersPage renders warehouse inventory transfers (سجلات حركة المخازن).
func (h *UIHandler) VendorTransfersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if _, ok := database.TenantFrom(ctx); !ok {
		if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
			ctx = database.WithTenant(ctx, actor.OrganizationID)
		}
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	if h.invSvc == nil {
		h.renderPage(ctx, w, "render vendor transfers fallback", pages.VendorTransfers(pages.VendorTransfersData{}, lang, dir, h.isHTMX(r)))
		return
	}

	transfers, totalCount, err := h.invSvc.ListTransfersWithTotal(ctx, "", limit, offset)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	whNames := make(map[int64]string)
	if whs, err := h.invSvc.ListWarehouses(ctx); err == nil {
		for _, wh := range whs {
			whNames[wh.ID] = wh.Name
		}
	}

	prodNames := make(map[int64]string)
	if h.catSvc != nil {
		for _, t := range transfers {
			if t.ProductID > 0 {
				if _, ok := prodNames[t.ProductID]; !ok {
					if p, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), t.ProductID); err == nil && p != nil {
						prodNames[t.ProductID] = p.Name.Get(i18n.AR)
					}
				}
			}
			if t.ProductVariantID > 0 {
				if _, ok := prodNames[t.ProductVariantID]; !ok {
					if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), t.ProductVariantID); err == nil && v != nil {
						prodNames[t.ProductVariantID] = v.Name.Get(i18n.AR)
					}
				}
			}
		}
	}

	data := pages.VendorTransfersData{
		Transfers:      transfers,
		WarehouseNames: whNames,
		ProductNames:   prodNames,
		NoticeType:     r.URL.Query().Get("notice_type"),
		NoticeMsg:      r.URL.Query().Get("notice"),
		Page:           page,
		PerPage:        limit,
		TotalCount:     totalCount,
	}

	h.renderPage(ctx, w, "render vendor transfers page", pages.VendorTransfers(data, lang, dir, h.isHTMX(r)))
}

// VendorStockTransferSubmit processes transferring stock from one warehouse to another.
func (h *UIHandler) VendorStockTransferSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/inventory", http.StatusSeeOther)
		return
	}

	redirectURL := r.PostFormValue("redirect_url")
	if redirectURL == "" {
		redirectURL = "/vendor/inventory"
	}

	if h.invSvc == nil {
		h.redirectWithNotice(w, r, redirectURL, "error", "خدمة المخزون غير متاحة حالياً.")
		return
	}

	fromWhID, _ := strconv.ParseInt(r.PostFormValue("from_warehouse_id"), 10, 64)
	toWhID, _ := strconv.ParseInt(r.PostFormValue("to_warehouse_id"), 10, 64)
	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	quantity, _ := strconv.Atoi(r.PostFormValue("quantity"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	immediate := r.PostFormValue("immediate") == "true" || r.PostFormValue("immediate") == "1"

	if fromWhID <= 0 || toWhID <= 0 {
		h.redirectWithNotice(w, r, redirectURL, "error", "يجب تحديد المخزن المصدر والمخزن الوجهة للتحويل.")
		return
	}
	if fromWhID == toWhID {
		h.redirectWithNotice(w, r, redirectURL, "error", "لا يمكن التحويل إلى نفس المخزن، يجب اختيار مخزن مختلف.")
		return
	}
	if quantity <= 0 {
		h.redirectWithNotice(w, r, redirectURL, "error", "يجب أن تكون الكمية المراد تحويلها أكبر من الصفر.")
		return
	}

	// Resolve product ID if variant is provided but product is missing
	if productID <= 0 && variantID > 0 && h.catSvc != nil {
		if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), variantID); err == nil && v != nil {
			productID = v.ProductID
		}
	}

	if productID <= 0 || variantID <= 0 {
		h.redirectWithNotice(w, r, redirectURL, "error", "تعذر تحديد بيانات الصنف المراد تحويله.")
		return
	}

	transferInput := &inventory.WarehouseTransfer{
		OrganizationID:   actor.OrganizationID,
		FromWarehouseID:  fromWhID,
		ToWarehouseID:    toWhID,
		ProductID:        productID,
		ProductVariantID: variantID,
		Quantity:         quantity,
		Notes:            notes,
		InitiatedBy:      &actor.UserID,
	}

	createdTransfer, err := h.invSvc.TransferStock(ctx, transferInput)
	if err != nil {
		h.redirectWithNotice(w, r, redirectURL, "error", "فشل تحويل المخزون: "+h.safeMessage(err, langOf(r)))
		return
	}

	if immediate && createdTransfer != nil {
		if _, err := h.invSvc.ReceiveTransfer(ctx, createdTransfer.ID); err != nil {
			h.redirectWithNotice(w, r, redirectURL, "success", "تم إرسال شحنة التحويل وهي قيد النقل بالمخزن.")
			return
		}
		h.redirectWithNotice(w, r, redirectURL, "success", fmt.Sprintf("تم نقل %d عبوة بنجاح وإيداعها في المخزن الوجهة.", quantity))
		return
	}

	h.redirectWithNotice(w, r, redirectURL, "success", fmt.Sprintf("تم إرسال شحنة التحويل (%d عبوة) وقيد النقل.", quantity))
}

// VendorTransferReceiveSubmit confirms receipt of an in-transit warehouse transfer.
func (h *UIHandler) VendorTransferReceiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/transfers", http.StatusSeeOther)
		return
	}

	transferID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if h.invSvc != nil && transferID > 0 {
		if _, err := h.invSvc.ReceiveTransfer(ctx, transferID); err != nil {
			h.redirectWithNotice(w, r, "/vendor/transfers", "error", "فشل تأكيد استلام الشحنة: "+h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/vendor/transfers", "success", "تم تأكيد استلام الشحنة بنجاح وإيداع الرصيد في المخزن.")
}

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
		_, _ = h.invSvc.AdjustStock(ctx, inventory.AdjustStockInput{
			StockID: stockID,
			Delta:   delta,
			Type:    inventory.MovementAdjustment,
			Details: r.PostFormValue("reason"),
			UserID:  &actor.UserID,
		})
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
