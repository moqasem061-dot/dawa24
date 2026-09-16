package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorProductsPage renders one page of the vendor's supply variants.
//
// Everything is paged, searched and filtered at the database. The screen this
// replaces asked for five hundred variants, rendered all of them, and filtered
// them in the browser — so a vendor with nine thousand could not reach the
// other eight and a half thousand, and the counters above the table told them
// they owned five hundred.
func (h *UIHandler) VendorProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	branchOptions, branchMap := h.vendorBranchOptions(ctx, actor.OrganizationID)
	query := vendorVariantQueryFrom(r)

	data := pages.VendorVariantsData{
		Branches:   branchOptions,
		Filter:     query,
		NoticeType: r.URL.Query().Get("notice_type"),
		NoticeMsg:  r.URL.Query().Get("notice"),
	}

	if h.catSvc != nil && actor.OrganizationID > 0 {
		variants, total, err := h.catSvc.ListVendorVariants(ctx, actor.OrganizationID, query)
		if err != nil {
			h.log.ErrorContext(ctx, "list vendor variants", "error", err)
			data.LoadError = h.safeMessage(err, langOf(r))
		} else {
			data.Total = total
			data.Variants = h.decorateVendorVariants(ctx, variants, branchMap)
		}

		stats, statsErr := h.catSvc.VendorVariantStats(ctx, actor.OrganizationID)
		if statsErr != nil {
			h.log.WarnContext(ctx, "vendor variant stats unavailable", "error", statsErr)
		} else {
			data.Stats = stats
		}
	}

	h.renderPage(ctx, w, "render vendor products page", pages.VendorProducts(data, lang, dir, h.isHTMX(r)))
}

// vendorVariantQueryFrom reads the listing controls out of the URL.
//
// Every value is clamped rather than trusted: the page size to the offered
// sizes, the page to at least one, so a hand-edited link cannot ask for a
// hundred thousand rows in one response.
func vendorVariantQueryFrom(r *http.Request) catalog.VendorVariantQuery {
	q := r.URL.Query()
	query := catalog.VendorVariantQuery{
		Query:      strings.TrimSpace(q.Get("q")),
		Status:     q.Get("status"),
		Stock:      catalog.StockFilter(q.Get("stock")),
		Expiring:   q.Get("expiring") == "1",
		Sort:       q.Get("sort"),
		PageNumber: 1,
		PerPage:    catalog.DefaultPageSize,
	}
	if n, err := strconv.Atoi(q.Get("page")); err == nil && n > 1 {
		query.PageNumber = n
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil {
		query.PerPage = n
	}
	switch query.Status {
	case string(catalog.StatusActive), string(catalog.StatusInactive),
		string(catalog.StatusPending), string(catalog.StatusRejected):
	default:
		query.Status = ""
	}
	switch query.Stock {
	case catalog.StockFilterIn, catalog.StockFilterLow, catalog.StockFilterOut:
	default:
		query.Stock = catalog.StockFilterAny
	}
	return query
}

// vendorBranchOptions lists the vendor's branches and a lookup for the table.
func (h *UIHandler) vendorBranchOptions(
	ctx context.Context, orgID int64,
) ([]pages.VendorBranchOption, map[int64]string) {
	names := make(map[int64]string)
	if h.orgSvc == nil || orgID <= 0 {
		return nil, names
	}
	branches, err := h.orgSvc.ListBranches(ctx, orgID)
	if err != nil {
		h.log.WarnContext(ctx, "vendor branches unavailable", "error", err)
		return nil, names
	}
	var options []pages.VendorBranchOption
	for _, b := range branches {
		name := b.Name.Get(i18n.AR)
		if name == "" {
			name = b.Name.Get(i18n.EN)
		}
		options = append(options, pages.VendorBranchOption{ID: b.ID, Name: name, IsMain: b.IsMain})
		names[b.ID] = name
	}
	return options, names
}

// decorateVendorVariants attaches the shared-catalogue product and the branch
// name to each row.
//
// The products are fetched in one query for the whole page. Fetching them per
// row is a hundred round trips for a hundred-row page, which is what the
// previous version did behind a cache that only helped when two variants of the
// same product happened to land on the same screen.
func (h *UIHandler) decorateVendorVariants(
	ctx context.Context, variants []*catalog.ProductVariant, branchMap map[int64]string,
) []*pages.VendorVariantView {
	ids := make([]int64, 0, len(variants))
	seen := make(map[int64]bool, len(variants))
	for _, v := range variants {
		if v.ProductID > 0 && !seen[v.ProductID] {
			seen[v.ProductID] = true
			ids = append(ids, v.ProductID)
		}
	}
	products, err := h.catSvc.ProductsByIDs(ctx, ids)
	if err != nil {
		h.log.WarnContext(ctx, "master products unavailable for vendor listing", "error", err)
		products = map[int64]*catalog.Product{}
	}

	out := make([]*pages.VendorVariantView, 0, len(variants))
	for _, v := range variants {
		branch := i18n.T("ar", "vendor.ingest.main_warehouse")
		if v.BranchID != nil {
			if name, ok := branchMap[*v.BranchID]; ok {
				branch = name
			}
		}
		out = append(out, &pages.VendorVariantView{
			Variant:       v,
			MasterProduct: products[v.ProductID],
			BranchName:    branch,
			StockQuantity: v.StockQty,
		})
	}
	return out
}

// VendorVariantDeleteSubmit removes a supplier's variant offer and clears associated warehouse stocks.
func (h *UIHandler) VendorVariantDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid variant ID", nil))
		return
	}

	if h.catSvc != nil {
		ctx = database.WithTenant(ctx, actor.OrganizationID)
		if err := h.catSvc.DeleteVariant(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.variant.delete_error_prefix")+h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/products", "success", i18n.T(langOf(r), "vendor.variant.deleted_success"))
}

// VendorVariantToggleStatusSubmit toggles a variant between active and inactive.
func (h *UIHandler) VendorVariantToggleStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)
	ctx = database.WithAuditActorID(ctx, actor.UserID)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(lang, "vendor.catalog.invalid_variant_id"))
		return
	}
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(lang, "common.catalog_service_unavailable"))
		return
	}

	back := vendorProductsBackURL(r)

	newStatus, err := h.catSvc.ToggleVariantStatus(ctx, actor.OrganizationID, id)
	if err != nil {
		h.log.ErrorContext(ctx, "toggle variant status", "error", err, "variant_id", id)
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.catalog.update_variant_error_prefix")+h.safeMessage(err, lang))
		return
	}

	msg := "تم تفعيل الصنف وإتاحته للطلب بالكتالوج بنجاح"
	if newStatus == catalog.StatusInactive {
		msg = "تم تعطيل الصنف وإيقاف ظهوره بالكتالوج بنجاح"
	}
	h.redirectWithNotice(w, r, back, "success", msg)
}
