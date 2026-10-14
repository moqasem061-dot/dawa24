package ui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// SmartOrderMatchSubmit updates or unmatches a line in smart order results.
func (h *UIHandler) SmartOrderMatchSubmit(w http.ResponseWriter, r *http.Request) {
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()

	productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if productID == 0 {
		productID, _ = strconv.ParseInt(r.FormValue("target_id"), 10, 64)
	}
	expectedRev, _ := strconv.Atoi(r.FormValue("expected_rev"))

	var updatedLine *smartorder.Line
	var matchErr error
	if h.smartOrderSvc != nil {
		updatedLine, matchErr = h.smartOrderSvc.CorrectMatch(r.Context(), run.OrganizationID, lineID, productID)
		if matchErr != nil {
			h.log.WarnContext(r.Context(), "smart order manual match failed", "line_id", lineID, "error", matchErr)
		}
	}

	h.smartOrderRecalculate(r, run)

	if h.isHTMX(r) && updatedLine != nil {
		if errors.Is(matchErr, matching.ErrRevisionConflict) || (expectedRev > 0 && updatedLine.MatchRevision > expectedRev) {
			updatedLine.ConflictNotice = RevisionConflictNoticeBilingual
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := pages.SmartOrderResultsData{
			Run: run,
		}
		h.renderPage(r.Context(), w, "render SmartOrderResultRow", pages.SmartOrderResultRow(data, updatedLine))
		return
	}

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
