package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
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
		if err != nil {
			h.logLoadErr(r.Context(), "smartOrderSvc.Candidates", err)
		} else {
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
		// A refusal at checkout usually means the world moved (coverage,
		// stock, quota). Bring the lines up to date so the buyer sees why.
		if fresh, gErr := h.smartOrderSvc.GetByID(ctx, run.OrganizationID, run.ID); gErr == nil {
			_ = h.smartOrderSvc.Refresh(ctx, fresh) // nolint:errswallow: best effort; the refusal itself is reported below
		}
		h.smartOrderBack(w, r, run, translateSmartOrderError(err, lang))
		return
	}
	if len(stale) > 0 {
		// The review screen re-renders with the changed lines named. Storing
		// them on the run would outlive their usefulness; the buyer resolves
		// them now or re-runs.
		h.smartOrderStale.put(run.PublicID, stale)
		// And the lines themselves are brought up to date, so the screen that
		// names what changed also shows each line's new supplier or outcome.
		fresh, loadErr := h.smartOrderSvc.GetByID(ctx, run.OrganizationID, run.ID)
		h.logLoadErr(ctx, "smart order reload after stale finalize", loadErr)
		if loadErr == nil && fresh != nil {
			if err := h.smartOrderSvc.Refresh(ctx, fresh); err != nil {
				h.log.WarnContext(ctx, "smart order refresh after stale finalize", "run_id", run.ID, "error", err)
			}
		}
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
		products, err := h.searchMasterCatalog(ctx, q)
		if err != nil {
			h.logLoadErr(ctx, "searchMasterCatalog", err)
		} else {
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
