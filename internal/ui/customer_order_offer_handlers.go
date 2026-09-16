package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerOrderLineOfferDetails returns the HTML modal partial containing the offer details
// for a customer viewing an order line (WO-16).
func (h *UIHandler) CustomerOrderLineOfferDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	orderIDStr := chi.URLParam(r, "id")
	lineIDStr := chi.URLParam(r, "lineID")

	orderID, err1 := strconv.ParseInt(orderIDStr, 10, 64)
	lineID, err2 := strconv.ParseInt(lineIDStr, 10, 64)
	if err1 != nil || err2 != nil || orderID <= 0 || lineID <= 0 {
		http.Error(w, "invalid parameters", http.StatusBadRequest)
		return
	}

	if h.commSvc == nil {
		http.Error(w, "commerce service unavailable", http.StatusServiceUnavailable)
		return
	}

	details, err := h.commSvc.GetOfferDetailsForOrderLine(ctx, orderID, lineID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to fetch offer details for order line",
			"order_id", orderID, "line_id", lineID, "error", err)
		http.Error(w, fmt.Sprintf("تعذر جلب تفاصيل العرض: %v", err), http.StatusNotFound)
		return
	}

	lang := langOf(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerOrderOfferModal(details, lang).Render(ctx, w)
}

// VendorOrderLineOfferDetails returns the HTML modal partial containing the offer details
// for a vendor viewing an order line (WO-16).
func (h *UIHandler) VendorOrderLineOfferDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	orderIDStr := chi.URLParam(r, "id")
	lineIDStr := chi.URLParam(r, "lineID")

	orderID, err1 := strconv.ParseInt(orderIDStr, 10, 64)
	lineID, err2 := strconv.ParseInt(lineIDStr, 10, 64)
	if err1 != nil || err2 != nil || orderID <= 0 || lineID <= 0 {
		http.Error(w, "invalid parameters", http.StatusBadRequest)
		return
	}

	if h.commSvc == nil {
		http.Error(w, "commerce service unavailable", http.StatusServiceUnavailable)
		return
	}

	details, err := h.commSvc.GetOfferDetailsForOrderLine(ctx, orderID, lineID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to fetch offer details for vendor order line",
			"order_id", orderID, "line_id", lineID, "error", err)
		http.Error(w, fmt.Sprintf("تعذر جلب تفاصيل العرض: %v", err), http.StatusNotFound)
		return
	}

	lang := langOf(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerOrderOfferModal(details, lang).Render(ctx, w)
}
