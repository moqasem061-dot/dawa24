package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminProductDetailPage renders detail view of a master catalog product.
func (h *UIHandler) AdminProductDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	prodID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || prodID <= 0 {
		http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
		return
	}

	var prod *catalog.Product
	var variants []*catalog.ProductVariant
	if h.catSvc != nil {
		prod, variants, _ = h.catSvc.GetProduct(database.AsSystem(ctx), prodID)
	}

	if prod == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", i18n.T(lang, "admin.products.not_found"))
		return
	}

	orgNames := make(map[int64]string)
	branchNames := make(map[int64]string)
	warehouseNames := make(map[int64]string)

	if h.catSvc != nil {
		variantRows, _, _ := h.catSvc.ListAdminVariantRows(database.AsSystem(ctx), catalog.AdminVariantFilter{
			ProductID: prodID,
			Limit:     200,
		})

		rowByVariantID := make(map[int64]*catalog.AdminVariantRow, len(variantRows))
		for _, row := range variantRows {
			if row != nil {
				rowByVariantID[row.VariantID] = row
			}
		}

		for _, v := range variants {
			if v == nil {
				continue
			}
			if row, ok := rowByVariantID[v.ID]; ok {
				v.StockQty = row.TotalQuantity
				if bName := row.BranchName.Get(i18n.Lang(lang)); bName != "" {
					branchNames[v.ID] = bName
				} else if bName := row.BranchName.Get(i18n.AR); bName != "" {
					branchNames[v.ID] = bName
				}
				if len(row.Warehouses) > 0 {
					var whParts []string
					for _, wh := range row.Warehouses {
						if wh.Quantity > 0 {
							whParts = append(whParts, fmt.Sprintf("%s (%d)", wh.WarehouseName, wh.Quantity))
						} else {
							whParts = append(whParts, wh.WarehouseName)
						}
					}
					warehouseNames[v.ID] = strings.Join(whParts, "، ")
				}
				if row.DisplayOrgName(lang) != "" {
					orgNames[v.OrganizationID] = row.DisplayOrgName(lang)
				}
			}
		}
	}

	if h.orgSvc != nil {
		for _, v := range variants {
			if v != nil && v.OrganizationID > 0 {
				if _, exists := orgNames[v.OrganizationID]; !exists {
					if o, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), v.OrganizationID); err == nil && o != nil {
						name := o.TradeName.Get("ar")
						if name == "" {
							name = o.LegalName
						}
						if name == "" {
							name = o.Name.Get("ar")
						}
						if name == "" {
							name = o.TradeName.Get("en")
						}
						if name == "" {
							name = o.Name.Get("en")
						}
						if name != "" {
							orgNames[v.OrganizationID] = name
						}
					}
				}
			}
		}
	}

	h.renderPage(ctx, w, "render admin product detail", pages.AdminProductDetailPage(prod, variants, orgNames, branchNames, warehouseNames, lang, dir))
}

// AdminProductChildrenPage lists every supplier's stock, with the branch it
// sits on and the warehouses holding it.
//
// One query answers the page. It used to load five hundred organisations and a
// thousand master products into maps on every request purely to resolve display
// names, which was both unbounded and wrong past those limits: a supplier at
// position 501 rendered with a blank name.
func (h *UIHandler) AdminProductChildrenPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	q := r.URL.Query()
	status := strings.TrimSpace(q.Get("status"))
	if status == "all" {
		status = ""
	}
	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)

	data := pages.AdminProductChildrenData{
		SearchQuery:  strings.TrimSpace(q.Get("q")),
		StatusFilter: status,
		StockFilter:  strings.TrimSpace(q.Get("stock")),
		ExpiringSoon: q.Get("expiring") == "1",
		Page:         page,
		PerPage:      limit,
	}
	data.OrganizationID = parseIDParam(q.Get("org_id"))
	data.BranchID = parseIDParam(q.Get("branch_id"))
	data.WarehouseID = parseIDParam(q.Get("warehouse_id"))

	if h.catSvc == nil {
		h.renderPage(ctx, w, "render admin product children", pages.AdminProductChildrenPage(data, lang, dir))
		return
	}

	sysCtx := database.AsSystem(ctx)
	rows, total, err := h.catSvc.ListAdminVariantRows(sysCtx, catalog.AdminVariantFilter{
		Query:          data.SearchQuery,
		Status:         data.StatusFilter,
		OrganizationID: data.OrganizationID,
		BranchID:       data.BranchID,
		WarehouseID:    data.WarehouseID,
		Stock:          data.StockFilter,
		ExpiringSoon:   data.ExpiringSoon,
		Limit:          limit,
		Offset:         (page - 1) * limit,
	})
	if err != nil {
		h.log.ErrorContext(ctx, "list supplier variants", "error", err)
		h.renderError(w, r, err)
		return
	}
	data.Rows = rows
	data.Total = total

	// The filter bar offers only values that actually carry stock, so a filter
	// can never select an empty result by naming something irrelevant.
	if opts, optErr := h.catSvc.AdminVariantFilterOptions(sysCtx); optErr == nil {
		data.Options = opts
	} else {
		h.log.WarnContext(ctx, "load supplier variant filter options", "error", optErr)
	}

	h.renderPage(ctx, w, "render admin product children", pages.AdminProductChildrenPage(data, lang, dir))
}

// parseIDParam reads an optional positive id from the query string. Anything
// else is no filter at all rather than a filter on zero.
func parseIDParam(raw string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}
func (h *UIHandler) AdminProductChildStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && id > 0 && h.catSvc != nil {
		sysCtx := database.AsSystem(database.WithAuditActorID(ctx, actor.UserID))
		variant, err := h.catSvc.GetVariant(sysCtx, id)
		if err == nil && variant != nil {
			newStatus := r.URL.Query().Get("status")
			if newStatus == "" {
				newStatus = r.PostFormValue("status")
			}
			if newStatus == "" {
				if variant.Status == catalog.StatusActive {
					newStatus = "inactive"
				} else {
					newStatus = "active"
				}
			}
			variant.Status = catalog.ProductStatus(newStatus)
			sysCtx = database.WithTenant(sysCtx, variant.OrganizationID)
			if _, err := h.catSvc.UpdateVariant(sysCtx, id, variant); err != nil {
				_, _ = h.catSvc.ToggleVariantStatus(sysCtx, variant.OrganizationID, id)
			}
		}
	}
	h.redirectWithNotice(w, r, "/admin/product-child", "success", i18n.T(langOf(r), "admin.catalog.variant_status_updated_success"))
}

// AdminStocksPage renders inventory stocks across all warehouses.
func (h *UIHandler) AdminStocksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var stocks []*inventory.Stock
	var total int
	if h.invSvc != nil {
		stocks, total, _ = h.invSvc.ListLowStockWithTotal(database.AsSystem(ctx), limit, offset)
	}

	h.renderPage(ctx, w, "render admin stocks page", pages.AdminStocksPage(stocks, lang, dir, page, limit, total))
}

// AdminProductsDeleteAllSubmit removes all master products and variants (Super Admin).
func (h *UIHandler) AdminProductsDeleteAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", i18n.T(lang, "admin.import.service_unavailable"))
		return
	}
	count, err := h.catSvc.DeleteAllProducts(database.AsSystem(ctx))
	if err != nil {
		h.log.ErrorContext(ctx, "delete all master products error", "error", err)
		h.redirectWithNotice(w, r, "/admin/products", "error", fmt.Sprintf(i18n.T(lang, "admin.catalog.delete_all_failed_format"), h.safeMessage(err, lang)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/products", "success", fmt.Sprintf(i18n.T(lang, "admin.catalog.deleted_all_success_format"), count))
}
