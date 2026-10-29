package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
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
		allWhs, loadErr := h.invSvc.ListWarehouses(ctx)
		h.logLoadErr(ctx, "invSvc.ListWarehouses", loadErr)
		for _, wh := range allWhs {
			if wh.OrganizationID == actor.OrganizationID {
				warehouses = append(warehouses, wh)
			}
		}

		allOrgStocks, loadErr := h.invSvc.ListStocksByOrg(ctx, actor.OrganizationID)
		h.logLoadErr(ctx, "invSvc.ListStocksByOrg", loadErr)
		for _, s := range allOrgStocks {
			if s.Quantity <= s.MinThreshold {
				allLowStocks = append(allLowStocks, s)
			}
		}

		if loaded, count, err := h.invSvc.ListLowStocksByOrgWithTotal(ctx, actor.OrganizationID, whID, q, statusFilter, limit, offset); err != nil {
			h.renderError(w, r, err)
			return
		} else {
			pagedStocks, total = loaded, count
		}

	}

	if h.orgSvc != nil {
		if loaded, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err != nil {
			h.logLoadErr(ctx, "orgSvc.ListBranches", err)
		} else {
			branches = loaded
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
			vMap, loadErr := h.catSvc.ListVariantsByProducts(ctx, productIDs)
			h.logLoadErr(ctx, "catSvc.ListVariantsByProducts", loadErr)
			for _, vList := range vMap {
				variants = append(variants, vList...)
			}
		}
	}

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	noticeType, noticeMessage := noticeFrom(r)
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
		NoticeType:   noticeType,
		NoticeMsg:    noticeMessage,
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
		if loaded, loaded2, err := h.invSvc.ListStocksByOrgWithTotal(ctx, actor.OrganizationID, whID, q, limit, offset); err != nil {
			h.logLoadErr(ctx, "invSvc.ListStocksByOrgWithTotal", err)
		} else {
			pagedStocks, total = loaded, loaded2
		}
		if loaded, err := h.invSvc.ListStocksByOrg(ctx, actor.OrganizationID); err != nil {
			h.logLoadErr(ctx, "invSvc.ListStocksByOrg", err)
		} else {
			allStocks = loaded
		}
		allWhs, loadErr := h.invSvc.ListWarehouses(ctx)
		h.logLoadErr(ctx, "invSvc.ListWarehouses", loadErr)
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
			vMap, loadErr := h.catSvc.ListVariantsByProducts(ctx, productIDs)
			h.logLoadErr(ctx, "catSvc.ListVariantsByProducts", loadErr)
			for _, vList := range vMap {
				variants = append(variants, vList...)
			}
		}
	}

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	noticeType, noticeMessage := noticeFrom(r)
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
		NoticeType:  noticeType,
		NoticeMsg:   noticeMessage,
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
	if whs, err := h.invSvc.ListWarehouses(ctx); err != nil {
		h.logLoadErr(ctx, "invSvc.ListWarehouses", err)
	} else {
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
