package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SavingProductsPreviewResponse represents the preview payload returned to UI.
type SavingProductsPreviewResponse struct {
	Success    bool               `json:"success"`
	Error      string             `json:"error,omitempty"`
	Headers    []string           `json:"headers"`
	Detected   SavingDetectedCols `json:"detected"`
	SampleRows [][]string         `json:"sample_rows"`
}

// handleSavingProductsPage renders the live price-delta tracking list for customer or vendor.
func (h *UIHandler) handleSavingProductsPage(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/%s/saving-products", audience), http.StatusSeeOther)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if search == "" {
		search = strings.TrimSpace(r.URL.Query().Get("q"))
	}
	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	if filter == "" {
		filter = "all"
	}

	var items []*catalog.SavingProductEnriched
	var stats *catalog.SavingProductStats

	limit := h.pageLimit(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	if h.catSvc != nil {
		it, st, err := h.catSvc.ListSavingProductsEnriched(ctx, actor.OrganizationID, search, filter, limit, offset)
		if err == nil {
			items = it
			stats = st
		} else {
			h.log.ErrorContext(ctx, fmt.Sprintf("%s list saving products enriched error", audience), "error", err, "org_id", actor.OrganizationID)
		}
	}

	if stats == nil {
		stats = &catalog.SavingProductStats{}
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	totalCount := stats.FilteredCount
	if totalCount == 0 && search == "" {
		switch filter {
		case "linked":
			totalCount = stats.CountLinked
		case "unlinked":
			totalCount = stats.CountUnlinked
		default:
			totalCount = stats.CountAll
		}
	}

	baseURL := fmt.Sprintf("/%s/saving-products", audience)
	paginationProps := components.PaginationProps{
		CurrentPage: page,
		PageSize:    limit,
		TotalCount:  totalCount,
		BaseURL:     baseURL,
		QueryValues: r.URL.Query(),
	}

	if audience == "vendor" {
		pageData := pages.VendorSavingPageData{
			Items:        items,
			Stats:        stats,
			SearchQuery:  search,
			FilterStatus: filter,
			NoticeType:   noticeType,
			NoticeMsg:    noticeMsg,
			Pagination:   paginationProps,
		}
		h.renderPage(ctx, w, "render vendor saving products", pages.VendorSavingProductsPage(pageData, lang, dir))
		return
	}

	pageData := pages.CustomerSavingPageData{
		Items:        items,
		Stats:        stats,
		SearchQuery:  search,
		FilterStatus: filter,
		NoticeType:   noticeType,
		NoticeMsg:    noticeMsg,
		Pagination:   paginationProps,
	}
	h.renderPage(ctx, w, "render customer saving products", pages.CustomerSavingProductsPage(pageData, lang, dir))
}

// handleSavingProductCreateSubmit handles manual creation of a saving product for customer or vendor.
func (h *UIHandler) handleSavingProductCreateSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	targetURL := fmt.Sprintf("/%s/saving-products", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetURL), http.StatusSeeOther)
		return
	}

	var formInvalidMsg string
	if audience == "vendor" {
		formInvalidMsg = i18n.T(langOf(r), "common.form_invalid")
	} else {
		formInvalidMsg = i18n.T(langOf(r), "customer.saving.form_invalid")
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, targetURL, "error", formInvalidMsg)
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.name_required"))
		return
	}

	sku := strings.TrimSpace(r.FormValue("sku"))
	qty, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("qty")), 64)
	price, _ := money.Parse(strings.TrimSpace(r.FormValue("price")))

	var productID *int64
	if prodStr := strings.TrimSpace(r.FormValue("product_id")); prodStr != "" {
		if pid, err := strconv.ParseInt(prodStr, 10, 64); err == nil && pid > 0 {
			productID = &pid
		}
	}

	sp := &catalog.SavingProduct{
		OrganizationID: actor.OrganizationID,
		UserID:         &actor.UserID,
		ProductID:      productID,
		NameProduct:    nameProduct,
		SKU:            sku,
		Quantity:       qty,
		Price:          price,
	}

	if h.catSvc != nil {
		if err := h.catSvc.CreateSavingProduct(ctx, sp); err != nil {
			h.log.ErrorContext(ctx, fmt.Sprintf("create %s saving product error", audience), "error", err)
			var errMsg string
			if audience == "customer" {
				errMsg = fmt.Sprintf(i18n.T(langOf(r), "customer.saving.create_error"), h.safeMessage(err, langOf(r)))
			} else {
				errMsg = h.safeMessage(err, langOf(r))
			}
			h.redirectWithNotice(w, r, targetURL, "error", errMsg)
			return
		}
	}

	var successMsg string
	if audience == "vendor" {
		successMsg = i18n.T(langOf(r), "vendor.saving.create_success")
	} else {
		successMsg = i18n.T(langOf(r), "customer.saving.create_success")
	}
	h.redirectWithNotice(w, r, targetURL, "success", successMsg)
}

// handleSavingProductUpdateSubmit handles updating an existing saving product for customer or vendor.
func (h *UIHandler) handleSavingProductUpdateSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	targetURL := fmt.Sprintf("/%s/saving-products", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetURL), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		var idErrMsg string
		if audience == "vendor" {
			idErrMsg = i18n.T(langOf(r), "vendor.saving.invalid_product_id")
		} else {
			idErrMsg = i18n.T(langOf(r), "customer.saving.invalid_id")
		}
		h.redirectWithNotice(w, r, targetURL, "error", idErrMsg)
		return
	}

	if err := r.ParseForm(); err != nil {
		var formErrMsg string
		if audience == "vendor" {
			formErrMsg = i18n.T(langOf(r), "common.form_invalid")
		} else {
			formErrMsg = i18n.T(langOf(r), "customer.saving.form_invalid")
		}
		h.redirectWithNotice(w, r, targetURL, "error", formErrMsg)
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.name_required"))
		return
	}

	sku := strings.TrimSpace(r.FormValue("sku"))
	qty, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("qty")), 64)
	price, _ := money.Parse(strings.TrimSpace(r.FormValue("price")))

	var productID *int64
	if prodStr := strings.TrimSpace(r.FormValue("product_id")); prodStr != "" {
		if pid, err := strconv.ParseInt(prodStr, 10, 64); err == nil && pid > 0 {
			productID = &pid
		}
	}

	sp := &catalog.SavingProduct{
		ID:             id,
		OrganizationID: actor.OrganizationID,
		UserID:         &actor.UserID,
		ProductID:      productID,
		NameProduct:    nameProduct,
		SKU:            sku,
		Quantity:       qty,
		Price:          price,
	}

	if h.catSvc != nil {
		if err := h.catSvc.UpdateSavingProduct(ctx, sp); err != nil {
			h.log.ErrorContext(ctx, fmt.Sprintf("update %s saving product error", audience), "error", err)
			var errMsg string
			if audience == "customer" {
				errMsg = fmt.Sprintf(i18n.T(langOf(r), "customer.saving.update_error"), h.safeMessage(err, langOf(r)))
			} else {
				errMsg = h.safeMessage(err, langOf(r))
			}
			h.redirectWithNotice(w, r, targetURL, "error", errMsg)
			return
		}
	}

	var successMsg string
	if audience == "vendor" {
		successMsg = i18n.T(langOf(r), "vendor.saving.update_success")
	} else {
		successMsg = i18n.T(langOf(r), "customer.saving.update_success")
	}
	h.redirectWithNotice(w, r, targetURL, "success", successMsg)
}

// handleSavingProductDeleteSubmit deletes a saving product record for customer or vendor.
func (h *UIHandler) handleSavingProductDeleteSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	targetURL := fmt.Sprintf("/%s/saving-products", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetURL), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		var idErrMsg string
		if audience == "vendor" {
			idErrMsg = i18n.T(langOf(r), "vendor.saving.invalid_product_id")
		} else {
			idErrMsg = i18n.T(langOf(r), "customer.saving.invalid_id")
		}
		h.redirectWithNotice(w, r, targetURL, "error", idErrMsg)
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.DeleteSavingProduct(ctx, id, actor.OrganizationID); err != nil {
			h.log.ErrorContext(ctx, fmt.Sprintf("delete %s saving product error", audience), "error", err)
			var errMsg string
			if audience == "customer" {
				errMsg = i18n.T(langOf(r), "customer.saving.delete_error")
			} else {
				errMsg = h.safeMessage(err, langOf(r))
			}
			h.redirectWithNotice(w, r, targetURL, "error", errMsg)
			return
		}
	}

	var successMsg string
	if audience == "vendor" {
		successMsg = i18n.T(langOf(r), "vendor.saving.delete_success")
	} else {
		successMsg = i18n.T(langOf(r), "customer.saving.delete_success")
	}
	h.redirectWithNotice(w, r, targetURL, "success", successMsg)
}
