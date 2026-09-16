package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// Review-time actions.
//
// Each one edits the run and returns the buyer to the review screen, where the
// recomputed totals are visible immediately (FR-044). They are form posts rather
// than fetch calls so the screen works without JavaScript — a pharmacy on a slow
// connection with an old browser still has to be able to place an order.

// SetFinalizer wires the finalisation use case.
func (h *UIHandler) SetFinalizer(f *smartorder.Finalizer) { h.smartOrderFinalizer = f }

// SmartOrderQuantitySubmit applies a quantity edit.
func (h *UIHandler) SmartOrderQuantitySubmit(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	qtyStr := strings.TrimSpace(r.FormValue("quantity"))
	qty, err := strconv.ParseFloat(qtyStr, 64)
	if err != nil || qty < 0 {
		h.smartOrderBack(w, r, run, i18n.T(lang, "smartorder.invalid_quantity"))
		return
	}
	if sel, err := h.smartOrderSvc.Selection(r.Context(), run.OrganizationID, lineID); err == nil && sel != nil {
		candidates, err := h.smartOrderSvc.Candidates(r.Context(), run.OrganizationID, lineID)
		if err == nil {
			for _, c := range candidates {
				if c.ID == sel.CandidateID && c.StockQty > 0 && qty > float64(c.StockQty) {
					h.smartOrderBack(w, r, run, fmt.Sprintf("الكمية المطلوبة (%g) تتجاوز المخزون المتاح لدى المورد (%d)", qty, c.StockQty))
					return
				}
			}
		}
	}
	if err := h.smartOrderSvc.SetQuantity(r.Context(), run.OrganizationID, lineID, qty); err != nil {
		h.smartOrderBack(w, r, run, translateSmartOrderError(err, lang))
		return
	}
	h.smartOrderRecalculate(r, run)
	h.smartOrderBack(w, r, run, "")
}

// SmartOrderSupplierSubmit switches a line to a different vendor.
func (h *UIHandler) SmartOrderSupplierSubmit(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	candidateID, err := strconv.ParseInt(r.FormValue("candidate_id"), 10, 64)
	if err != nil {
		h.smartOrderBack(w, r, run, i18n.T(lang, "smartorder.select_supplier_from_list"))
		return
	}
	if err := h.smartOrderSvc.ChooseSupplier(r.Context(), run.OrganizationID, lineID, candidateID); err != nil {
		h.smartOrderBack(w, r, run, translateSmartOrderError(err, lang))
		return
	}
	h.smartOrderRecalculate(r, run)
	h.smartOrderBack(w, r, run, "")
}

// SmartOrderRemoveSubmit drops a line from the order.
func (h *UIHandler) SmartOrderRemoveSubmit(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	if err := h.smartOrderSvc.RemoveLine(r.Context(), run.OrganizationID, lineID); err != nil {
		h.smartOrderBack(w, r, run, translateSmartOrderError(err, lang))
		return
	}
	h.smartOrderRecalculate(r, run)
	h.smartOrderBack(w, r, run, "")
}

// SmartOrderBulkRemoveSubmit drops multiple selected lines from the order at once.
func (h *UIHandler) SmartOrderBulkRemoveSubmit(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		h.smartOrderBack(w, r, run, i18n.T(lang, "smartorder.form_parse_error"))
		return
	}

	lineIDs := r.Form["line_ids"]
	if len(lineIDs) == 0 {
		lineIDs = r.Form["line_id"]
	}
	if len(lineIDs) == 0 {
		h.smartOrderBack(w, r, run, "لم يتم تحديد أي أصناف لحذفها")
		return
	}

	for _, idStr := range lineIDs {
		lineID, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil || lineID <= 0 {
			continue
		}
		if err := h.smartOrderSvc.RemoveLine(r.Context(), run.OrganizationID, lineID); err != nil {
			h.log.WarnContext(r.Context(), "bulk remove line failed", "line_id", lineID, "error", err)
		}
	}
	h.smartOrderRecalculate(r, run)
	h.smartOrderBack(w, r, run, "")
}

// SmartOrderDefaultQuantitySubmit applies a default quantity to all or selected lines.
func (h *UIHandler) SmartOrderDefaultQuantitySubmit(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		_ = r.ParseMultipartForm(32 << 20)
	} else {
		_ = r.ParseForm()
	}

	qtyStr := strings.TrimSpace(r.FormValue("quantity"))
	qty, err := strconv.ParseFloat(qtyStr, 64)
	if err != nil || qty <= 0 {
		h.smartOrderBack(w, r, run, i18n.T(lang, "smartorder.invalid_quantity"))
		return
	}

	lineIDStrs := r.Form["line_ids"]
	if len(lineIDStrs) == 0 {
		lineIDStrs = r.Form["line_id"]
	}
	var lineIDs []int64
	for _, s := range lineIDStrs {
		if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && id > 0 {
			lineIDs = append(lineIDs, id)
		}
	}

	if len(lineIDs) == 0 {
		orderable, _, err := h.smartOrderSvc.Results(r.Context(), run, smartorder.LineFilter{
			Outcome: string(smartorder.OutcomeOrdered), All: true,
		})
		if err == nil && len(orderable) == 0 {
			h.smartOrderBack(w, r, run, i18n.T(lang, "smartorder.err_nothing_to_order"))
			return
		}
	}

	if err := h.smartOrderSvc.SetDefaultQuantity(r.Context(), run.OrganizationID, run.ID, lineIDs, qty); err != nil {
		h.smartOrderBack(w, r, run, translateSmartOrderError(err, lang))
		return
	}

	h.smartOrderRecalculate(r, run)
	h.smartOrderBack(w, r, run, "")
}


// SmartOrderFinalizeSubmit re-verifies every line and places the order.
//
// A line that changed since generation stops the whole order and is named on the
// review screen. Nothing is substituted and nothing is dropped: a pharmacy that
// ordered from one supplier and silently received from another finds out at
// delivery (FR-047).
func (h *UIHandler) SmartOrderFinalizeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}
	if h.smartOrderFinalizer == nil {
		h.smartOrderBack(w, r, run, i18n.T(lang, "smartorder.finalizer_unavailable"))
		return
	}

	orderID, stale, err := h.smartOrderFinalizer.Finalize(ctx, run)
	if err != nil {
		h.log.WarnContext(ctx, "smart order finalize failed", "run_id", run.ID, "error", err)
		h.smartOrderBack(w, r, run, translateSmartOrderError(err, lang))
		return
	}
	if len(stale) > 0 {
		// The review screen re-renders with the changed lines named. Storing
		// them on the run would outlive their usefulness; the buyer resolves
		// them now or re-runs.
		h.smartOrderStale.put(run.PublicID, stale)
		http.Redirect(w, r, "/customer/smart-order/"+run.PublicID+"/review", http.StatusSeeOther)
		return
	}

	h.smartOrderStale.drop(run.PublicID)
	http.Redirect(w, r, "/customer/orders/"+strconv.FormatInt(orderID, 10), http.StatusSeeOther)
}

// smartOrderLineAction resolves the run and line for a review edit.
func (h *UIHandler) smartOrderLineAction(w http.ResponseWriter, r *http.Request) (*smartorder.Run, int64, bool) {
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return nil, 0, false
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		_ = r.ParseMultipartForm(32 << 20)
	} else {
		if err := r.ParseForm(); err != nil {
			h.smartOrderBack(w, r, run, i18n.T(langOf(r), "smartorder.form_parse_error"))
			return nil, 0, false
		}
	}
	lineID, err := strconv.ParseInt(chi.URLParam(r, "lineID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil, 0, false
	}
	return run, lineID, true
}

// smartOrderRecalculate refreshes the run totals after an edit.
//
// A failure here is logged rather than surfaced: the edit itself succeeded, and
// the next page render recomputes anyway.
func (h *UIHandler) smartOrderRecalculate(r *http.Request, run *smartorder.Run) {
	ctx := r.Context()
	cfg, err := h.smartOrderSvc.Config(ctx, run.ID)
	if err != nil {
		h.log.WarnContext(ctx, "could not load config to recalculate", "run_id", run.ID, "error", err)
		return
	}
	if updated, err := h.smartOrderSvc.Recalculate(ctx, run, cfg); err != nil {
		h.log.WarnContext(ctx, "could not recalculate smart order totals", "run_id", run.ID, "error", err)
	} else {
		run.EstimatedTotal = updated
	}
}

func (h *UIHandler) smartOrderBack(w http.ResponseWriter, r *http.Request, run *smartorder.Run, message string) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") || r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if message != "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": message})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":              true,
			"estimated_total": run.EstimatedTotal.String(),
			"run_id":          run.PublicID,
		})
		return
	}
	target := "/customer/smart-order/" + run.PublicID + "/review"
	q := url.Values{}
	if message != "" {
		q.Set("error", message)
	}
	if p := r.FormValue("page"); p != "" && p != "1" {
		q.Set("page", p)
	} else if p := r.URL.Query().Get("page"); p != "" && p != "1" {
		q.Set("page", p)
	}
	if l := r.FormValue("limit"); l != "" && l != "25" {
		q.Set("limit", l)
	} else if l := r.URL.Query().Get("limit"); l != "" && l != "25" {
		q.Set("limit", l)
	}
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// SmartOrderCatalogSearch returns JSON results for the in-cell modal/combobox.
func (h *UIHandler) SmartOrderCatalogSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if q == "" {
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}

	type catalogItem struct {
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

	var out []catalogItem
	if h.ingSvc != nil {
		products, err := h.ingSvc.SearchMasterCatalog(ctx, q)
		if err == nil {
			for _, p := range products {
				out = append(out, catalogItem{
					ID:            p.ID,
					NameAR:        p.Name.Get("ar"),
					NameEN:        p.Name.Get("en"),
					SKU:           p.SKU,
					Barcode:       p.Barcode,
					DosageForm:    p.DosageForm,
					Concentration: p.Concentration,
					Manufacturer:  p.ManufacturingCompanies,
					PublicPrice:   float64(p.Price.Minor()) / 100.0,
				})
			}
		}
	}

	_ = json.NewEncoder(w).Encode(out)
}

// SmartOrderMatchSubmit updates or unmatches a line in smart order results.
func (h *UIHandler) SmartOrderMatchSubmit(w http.ResponseWriter, r *http.Request) {
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()

	productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	var updatedLine *smartorder.Line
	if h.smartOrderSvc != nil {
		var err error
		updatedLine, err = h.smartOrderSvc.CorrectMatch(r.Context(), run.OrganizationID, lineID, productID)
		if err != nil {
			h.log.WarnContext(r.Context(), "smart order manual match failed", "line_id", lineID, "error", err)
		}
	}

	h.smartOrderRecalculate(r, run)

	if strings.Contains(r.Header.Get("Accept"), "application/json") || r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		resp := map[string]any{
			"ok":              true,
			"line_id":         lineID,
			"product_id":      productID,
			"estimated_total": run.EstimatedTotal.String(),
		}
		if updatedLine != nil {
			lang := langOf(r)
			resp["outcome"] = string(updatedLine.Outcome)
			resp["outcome_label"] = pages.SmartOrderOutcomeLabel(updatedLine.Outcome, lang)
			resp["is_ready"] = updatedLine.Outcome == smartorder.OutcomeOrdered
			resp["match_method"] = pages.MatchMethodLabel(updatedLine.MatchMethod, lang)
			resp["product_name"] = updatedLine.MatchedProductName
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	q := r.URL.Query()
	vals := url.Values{}
	for _, key := range []string{"match", "outcome", "sort", "order", "page", "limit", "q"} {
		if v := q.Get(key); v != "" {
			vals.Set(key, v)
		}
	}
	redirectURL := "/customer/smart-order/" + run.PublicID + "/results"
	if len(vals) > 0 {
		redirectURL += "?" + vals.Encode()
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// RegisterSmartOrderRoutes mounts the wizard on the shared buying surface.
//
// Every route here used to be registered flat, with no permission gate at all:
// membership of an approved pharmacy was the only question asked, so any
// employee could upload a list, re-match its lines, switch suppliers and place
// the resulting order — while pharmacy.smart_order.view and .run existed in the
// catalogue, appeared in the role editor, and controlled nothing. The route
// audit did not catch it because its glob only reads files named *routes*.go.
//
// Reading the wizard and running it are two grants. Uploading a list, changing
// what it matched, choosing a different supplier and finalising all commit the
// company to a purchase; opening the history does not.
func (h *UIHandler) RegisterSmartOrderRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuySmartOrderView))
		g.Get("/customer/smart-order", h.SmartOrderHistoryPage)
		g.Get("/customer/smart-order/history", h.SmartOrderHistoryPage)
		g.Get("/customer/smart-order/new", h.SmartOrderNewPage)
		g.Get("/customer/smart-order/sample.xlsx", h.SmartOrderSampleXLSX)
		g.Get("/customer/smart-order/sample.csv", h.SmartOrderSampleCSV)

		g.Get("/customer/smart-order/{id}/mapping", h.SmartOrderMappingPage)
		g.Get("/customer/smart-order/{id}/progress", h.SmartOrderProgressPage)
		g.Get("/customer/smart-order/{id}/progress.json", h.SmartOrderProgressJSON)
		// Both shapes, mirroring the two .json routes above: the progress page
		// and the run page each derive the stream URL from their own path, and
		// a client asking for a route that does not exist falls silently back
		// to polling — which looks like the stream simply never helping.
		g.Get("/customer/smart-order/{id}/progress/stream", h.SmartOrderProgressStream)
		g.Get("/customer/smart-order/{id}/stream", h.SmartOrderProgressStream)
		g.Get("/customer/smart-order/{id}.json", h.SmartOrderProgressJSON)
		g.Get("/customer/smart-order/{id}/results", h.SmartOrderResultsPage)
		g.Get("/customer/smart-order/{id}/catalog-search", h.SmartOrderCatalogSearch)
		g.Get("/customer/smart-order/{id}/review", h.SmartOrderReviewPage)
		g.Get("/customer/smart-order/{id}/export", h.SmartOrderExportCSV)
		// Legacy query-param redirects (e.g. /customer/smart-order/review?run_id=64 or ?id=...)
		g.Get("/customer/smart-order/review", h.SmartOrderLegacyRedirect)
		g.Get("/customer/smart-order/results", h.SmartOrderLegacyRedirect)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuySmartOrderRun))
		g.Post("/customer/smart-order", h.SmartOrderCreateSubmit)
		g.Post("/customer/smart-order/{id}/mapping", h.SmartOrderMappingSubmit)
		g.Post("/customer/smart-order/{id}/lines/{lineID}/quantity", h.SmartOrderQuantitySubmit)
		g.Post("/customer/smart-order/{id}/default-quantity", h.SmartOrderDefaultQuantitySubmit)
		g.Post("/customer/smart-order/{id}/lines/default-quantity", h.SmartOrderDefaultQuantitySubmit)
		g.Post("/customer/smart-order/{id}/lines/{lineID}/match", h.SmartOrderMatchSubmit)
		g.Post("/customer/smart-order/{id}/lines/{lineID}/supplier", h.SmartOrderSupplierSubmit)
		g.Post("/customer/smart-order/{id}/lines/{lineID}/remove", h.SmartOrderRemoveSubmit)
		g.Post("/customer/smart-order/{id}/lines/bulk-remove", h.SmartOrderBulkRemoveSubmit)
		g.Post("/customer/smart-order/{id}/finalize", h.SmartOrderFinalizeSubmit)
	})
}
