package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// loadImportReview fills the review table with pagination and filters.
func (h *UIHandler) loadImportReview(r *http.Request, view *pages.VendorImportView) {
	ctx := r.Context()
	limit := 25
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && (l == 10 || l == 25 || l == 50 || l == 100) {
		limit = l
	}
	matchLevel := r.URL.Query().Get("match")
	if matchLevel == "" {
		matchLevel = r.URL.Query().Get("filter")
	}
	filter := ingest.RowFilter{
		Outcome:    r.URL.Query().Get("outcome"),
		MatchLevel: matchLevel,
		Search:     r.URL.Query().Get("q"),
		SortBy:     r.URL.Query().Get("sort"),
		SortOrder:  r.URL.Query().Get("order"),
		Limit:      limit,
	}
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 1 {
		page = p
		filter.Offset = (page - 1) * filter.Limit
	}
	view.Filter = filter
	view.Page = page
	view.PerPage = limit

	rows, total, err := h.ingSvc.ImportRows(ctx, view.Session.PublicID, filter)
	if err != nil {
		h.log.WarnContext(ctx, "import review rows unavailable", "error", err)
		return
	}
	if view.Session != nil && len(rows) > 0 {
		_ = h.ingSvc.AnnotateRowsWithExistingVariants(ctx, view.Session, rows)
	}
	view.Rows, view.RowTotal = rows, total

	counts, err := h.ingSvc.ImportRowCounts(ctx, view.Session.PublicID)
	if err != nil {
		h.log.WarnContext(ctx, "import row counts unavailable", "error", err)
		return
	}
	view.RowCounts = counts

	// What committing would actually do, under the mode the vendor chose. The
	// screen used to print the matched count beside a prose description of the
	// mode, which is the right number for one mode out of four.
	plan, err := h.ingSvc.PreviewCommit(ctx, view.Session.PublicID)
	if err != nil {
		h.log.WarnContext(ctx, "import commit preview unavailable",
			"import", view.Session.PublicID, "error", err)
		return
	}
	view.Plan = &plan
}

// VendorIngestRowUpdateSubmit updates a staged row's variant name, price, quantity, etc.
func (h *UIHandler) VendorIngestRowUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	rowIDStr := chi.URLParam(r, "rowID")
	rowID, err := strconv.ParseInt(rowIDStr, 10, 64)
	if err != nil || h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", i18n.T(langOf(r), "vendor.ingest.invalid_row_id"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	customVariantName := strings.TrimSpace(r.PostFormValue("custom_variant_name"))
	displayName := strings.TrimSpace(r.PostFormValue("display_name"))

	var pricePtr *float64
	if pStr := strings.TrimSpace(r.PostFormValue("price")); pStr != "" {
		if pVal, pErr := strconv.ParseFloat(pStr, 64); pErr == nil {
			pricePtr = &pVal
		}
	}

	var discountPtr *float64
	if dStr := strings.TrimSpace(r.PostFormValue("discount")); dStr != "" {
		if dVal, dErr := strconv.ParseFloat(dStr, 64); dErr == nil {
			discountPtr = &dVal
		}
	}

	var costPricePtr *float64
	if cpStr := strings.TrimSpace(r.PostFormValue("cost_price")); cpStr != "" {
		if cpVal, cpErr := strconv.ParseFloat(cpStr, 64); cpErr == nil {
			costPricePtr = &cpVal
		}
	}

	var costDiscountPtr *float64
	if cdStr := strings.TrimSpace(r.PostFormValue("cost_discount")); cdStr != "" {
		if cdVal, cdErr := strconv.ParseFloat(cdStr, 64); cdErr == nil {
			costDiscountPtr = &cdVal
		}
	}

	var qtyPtr *int
	if qStr := strings.TrimSpace(r.PostFormValue("quantity")); qStr != "" {
		if qVal, qErr := strconv.Atoi(qStr); qErr == nil {
			qtyPtr = &qVal
		}
	}

	if err := h.ingSvc.UpdateStagedRow(ctx, publicID, rowID, displayName, customVariantName, pricePtr, discountPtr, costPricePtr, costDiscountPtr, qtyPtr, nil); err != nil {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "success", i18n.T(langOf(r), "vendor.ingest.item_updated_success"))
}

// VendorIngestBatchQuantitySubmit applies a uniform quantity to all staged items in the import.
func (h *UIHandler) VendorIngestBatchQuantitySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", i18n.T(langOf(r), "common.import_service_unavailable"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	qStr := strings.TrimSpace(r.PostFormValue("batch_quantity"))
	qty, err := strconv.Atoi(qStr)
	if err != nil || qty < 0 {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "error", i18n.T(langOf(r), "vendor.ingest.enter_valid_quantity"))
		return
	}

	if err := h.ingSvc.SetBatchQuantity(ctx, publicID, qty); err != nil {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "success", fmt.Sprintf(i18n.T(langOf(r), "vendor.ingest.batch_quantity_success"), qty))
}

// VendorIngestRowMatchSubmit manually assigns a master catalog product to a staged row.
func (h *UIHandler) VendorIngestRowMatchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	rowIDStr := chi.URLParam(r, "rowID")
	rowID, err := strconv.ParseInt(rowIDStr, 10, 64)
	if err != nil || h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", i18n.T(langOf(r), "vendor.ingest.invalid_row_id"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)

	if err := h.ingSvc.AssignStagedRowMatch(ctx, publicID, rowID, productID); err != nil {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "error", h.safeMessage(err, langOf(r)))
		return
	}

	if productID > 0 {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "success", i18n.T(langOf(r), "vendor.ingest.item_linked_success"))
	} else {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "success", i18n.T(langOf(r), "vendor.ingest.item_unlinked_success"))
	}
}

// VendorIngestRowToggleSubmit toggles whether a staged row will be included or excluded from commit.
func (h *UIHandler) VendorIngestRowToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	rowIDStr := chi.URLParam(r, "rowID")
	rowID, err := strconv.ParseInt(rowIDStr, 10, 64)
	if err != nil || h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", i18n.T(langOf(r), "vendor.ingest.invalid_row_id"))
		return
	}

	_, err = h.ingSvc.ToggleStagedRowExclude(ctx, publicID, rowID)
	if err != nil {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "error", h.safeMessage(err, langOf(r)))
		return
	}

	http.Redirect(w, r, buildReviewRedirect(publicID, r), http.StatusSeeOther)
}

// VendorIngestCatalogSearchJSON returns JSON search results from master catalog for modal picker.
func (h *UIHandler) VendorIngestCatalogSearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if h.ingSvc == nil || q == "" {
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}

	products, err := h.ingSvc.SearchMasterCatalog(ctx, q)
	if err != nil {
		http.Error(w, `{"error":"search_failed"}`, http.StatusInternalServerError)
		return
	}

	type searchItem struct {
		ID            int64   `json:"id"`
		NameAR        string  `json:"name_ar"`
		NameEN        string  `json:"name_en"`
		SKU           string  `json:"sku"`
		Barcode       string  `json:"barcode"`
		DosageForm    string  `json:"dosage_form"`
		Concentration string  `json:"concentration"`
		Manufacturer  string  `json:"manufacturer"`
		PublicPrice   float64 `json:"public_price"`
	}

	out := make([]searchItem, 0, len(products))
	for _, p := range products {
		nameAR := p.Name.Get("ar")
		nameEN := p.Name.Get("en")
		out = append(out, searchItem{
			ID:            p.ID,
			NameAR:        nameAR,
			NameEN:        nameEN,
			SKU:           p.SKU,
			Barcode:       p.Barcode,
			DosageForm:    p.DosageForm,
			Concentration: p.Concentration,
			Manufacturer:  p.ManufacturingCompanies,
			PublicPrice:   float64(p.Price.Minor()) / 100.0,
		})
	}

	_ = json.NewEncoder(w).Encode(out)
}

// VendorIngestBackToSettingsSubmit moves session back to settings phase.
func (h *UIHandler) VendorIngestBackToSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "common.import_service_unavailable"))
		return
	}

	if _, err := h.ingSvc.BackToSettings(ctx, publicID); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", h.safeMessage(err, langOf(r)))
		return
	}

	http.Redirect(w, r, "/vendor/ingest/"+publicID, http.StatusSeeOther)
}

// VendorIngestCommitSubmit starts the write of the reviewed rows.
//
// It starts it rather than performing it. The commit writes a variant and a
// balance per confirmed row, which on a real price list is tens of thousands of
// statements; doing that inside this POST raced the proxy timeout, and a vendor
// who navigated away cancelled the request context mid-write. The run is now
// detached and the vendor is sent to the progress screen, which polls the same
// endpoint the staging pass uses.
func (h *UIHandler) VendorIngestCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "common.import_service_unavailable"))
		return
	}

	if _, err := h.ingSvc.CommitInBackground(ctx, publicID); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", h.safeMessage(err, langOf(r)))
		return
	}
	http.Redirect(w, r, "/vendor/ingest/"+publicID, http.StatusSeeOther)
}

func buildReviewRedirect(publicID string, r *http.Request) string {
	q := r.URL.Query().Get("q")
	match := r.URL.Query().Get("match")
	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")
	page := r.URL.Query().Get("page")
	limit := r.URL.Query().Get("limit")

	url := "/vendor/ingest/" + publicID
	if orgID := chi.URLParam(r, "orgID"); orgID != "" {
		url = fmt.Sprintf("/admin/organizations/import/%s/ingest/%s", orgID, publicID)
	}
	params := []string{}
	if match != "" {
		params = append(params, "match="+match)
	}
	if sort != "" {
		params = append(params, "sort="+sort)
	}
	if order != "" {
		params = append(params, "order="+order)
	}
	if page != "" {
		params = append(params, "page="+page)
	}
	if limit != "" {
		params = append(params, "limit="+limit)
	}
	if q != "" {
		params = append(params, "q="+q)
	}
	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}
	return url
}
