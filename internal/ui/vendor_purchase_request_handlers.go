package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorPurchaseRequestsPage renders incoming purchase requests for a supplier (Plan V5 Phase 3 §3.1.5).
func (h *UIHandler) VendorPurchaseRequestsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/purchase-requests", http.StatusSeeOther)
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var requests []*commerce.PurchaseRequest
	var total int
	if h.commSvc != nil {
		reqs, tot, err := h.commSvc.ListVendorPurchaseRequestsWithTotal(ctx, actor.OrganizationID, status, limit, offset)
		if err == nil {
			requests = reqs
			total = tot
		}
	}

	var negOrders []*commerce.Order
	var negTotal int
	if h.commSvc != nil {
		orders, oTot, err := h.commSvc.ListVendorNegotiationOrdersWithTotal(ctx, actor.OrganizationID, status, limit, offset)
		if err == nil {
			negOrders = orders
			negTotal = oTot
		}
	}

	displayTotal := negTotal
	if displayTotal == 0 && total > 0 {
		displayTotal = total
	}

	h.renderPage(ctx, w, "render vendor purchase requests page", pages.VendorPurchaseRequestsPage(lang, dir, requests, negOrders, status, page, limit, displayTotal))
}

// VendorPurchaseRequestDetailPage renders one incoming purchase request with line items.
func (h *UIHandler) VendorPurchaseRequestDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/purchase-requests", http.StatusSeeOther)
		return
	}

	reqIDStr := chi.URLParam(r, "id")
	reqID, err := strconv.ParseInt(reqIDStr, 10, 64)
	if err != nil || reqID <= 0 {
		http.Redirect(w, r, "/vendor/purchase-requests", http.StatusSeeOther)
		return
	}

	var request *commerce.PurchaseRequest
	if h.commSvc != nil {
		req, err := h.commSvc.GetPurchaseRequest(ctx, reqID)
		if err == nil && req.VendorOrgID == actor.OrganizationID {
			request = req
		}
	}
	if request == nil {
		http.Redirect(w, r, "/vendor/purchase-requests", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render vendor purchase request detail page", pages.VendorPurchaseRequestDetailPage(lang, dir, request))
}

// VendorPurchaseRequestRespondSubmit allows vendor to approve, reject, or comment on a purchase request.
func (h *UIHandler) VendorPurchaseRequestRespondSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	reqIDStr := chi.URLParam(r, "id")
	reqID, err := strconv.ParseInt(reqIDStr, 10, 64)
	if err != nil || reqID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/purchase-requests", "error", i18n.T(lang, "vendor.purchase_request.invalid_id"))
		return
	}

	status := commerce.PurchaseRequestStatus(r.FormValue("status"))
	vendorNotes := strings.TrimSpace(r.FormValue("vendor_notes"))

	if h.commSvc != nil {
		if back, msg := h.respondVendorPurchaseRequest(ctx, actor, lang, reqID, status, vendorNotes); msg != "" {
			h.redirectWithNotice(w, r, back, "error", msg)
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/purchase-requests/"+reqIDStr, "success", i18n.T(lang, "vendor.purchase_request.status_updated_success"))
}

// VendorPurchaseRequestLineRespondSubmit updates vendor price/discount counter-offer for a specific line item.
func (h *UIHandler) VendorPurchaseRequestLineRespondSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	lineIDStr := chi.URLParam(r, "id")
	lineID, err := strconv.ParseInt(lineIDStr, 10, 64)
	if err != nil || lineID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/purchase-requests", "error", i18n.T(lang, "vendor.purchase_request.invalid_line_id"))
		return
	}

	var price money.Amount
	if pStr := r.FormValue("offered_price"); pStr != "" {
		if p, err := money.Parse(pStr); err == nil {
			price = p
		}
	}

	var discount float64
	if dStr := r.FormValue("offered_discount"); dStr != "" {
		if d, err := strconv.ParseFloat(dStr, 64); err == nil && d >= 0 {
			discount = d
		}
	}

	status := r.FormValue("status")
	if status == "" {
		status = "approved"
	}

	if h.commSvc != nil {
		if err := h.commSvc.UpdatePurchaseRequestLineOffer(ctx, lineID, price, discount, status); err != nil {
			h.redirectWithNotice(w, r, "/vendor/purchase-requests", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/purchase-requests", "success", i18n.T(lang, "vendor.purchase_request.line_offer_updated_success"))
}

// incomingPurchaseRequest loads a quotation request sent to this supplier.
func (h *UIHandler) incomingPurchaseRequest(ctx context.Context, actor authctx.Actor, lang string, reqID int64) (*commerce.PurchaseRequest, string) {
	req, err := h.commSvc.GetPurchaseRequest(ctx, reqID)
	if err != nil || req == nil {
		return nil, i18n.T(lang, "vendor.purchase_request.not_found")
	}
	if req.VendorOrgID != actor.OrganizationID {
		return nil, i18n.T(lang, "vendor.orders.unauthorized_order_management")
	}
	return req, ""
}

// respondVendorPurchaseRequest sets a request's status with the supplier's
// notes and tells the buyer. On failure it returns where to go back to and why.
func (h *UIHandler) respondVendorPurchaseRequest(
	ctx context.Context, actor authctx.Actor, lang string, reqID int64, status commerce.PurchaseRequestStatus, notes string,
) (string, string) {
	req, msg := h.incomingPurchaseRequest(ctx, actor, lang, reqID)
	if msg != "" {
		return "/vendor/purchase-requests", msg
	}
	responderID := actor.UserID
	if err := h.commSvc.RespondPurchaseRequest(ctx, reqID, status, notes, &responderID); err != nil {
		return "/vendor/purchase-requests/" + strconv.FormatInt(reqID, 10), h.safeMessage(err, lang)
	}
	vendorName := h.resolveOrgName(ctx, actor.OrganizationID)
	var custOrgID int64
	if req.OrganizationID != nil {
		custOrgID = *req.OrganizationID
	}
	h.safeGo("notify-purchase-request-responded", func() {
		h.notifyPurchaseRequestResponded(context.Background(), req.CustomerID, custOrgID, vendorName, reqID)
	})
	return "", ""
}
