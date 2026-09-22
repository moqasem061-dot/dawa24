package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/orders", http.StatusSeeOther)
		return
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	if h.commSvc == nil {
		h.renderPage(ctx, w, "render customer orders page", pages.CustomerOrders(pages.CustomerOrdersData{}, lang, dir, h.isHTMX(r)))
		return
	}

	orders, total, err := h.commSvc.ListCustomerOrdersWithTotal(ctx, userID, limit, offset)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	data := pages.CustomerOrdersData{
		Orders:     orders,
		Page:       page,
		PerPage:    limit,
		TotalCount: total,
	}

	h.renderPage(ctx, w, "render customer orders page", pages.CustomerOrders(data, lang, dir, h.isHTMX(r)))
}

func (h *UIHandler) CustomerOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if h.commSvc == nil {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}

	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/orders/%d", id), http.StatusSeeOther)
		return
	}

	order, err := h.commSvc.GetOrder(ctx, id)
	if err != nil || order == nil {
		h.redirectWithNotice(w, r, "/orders", "error", i18n.T(lang, "orders.order_not_found"))
		return
	}

	// Security & Tenant Isolation: verify actor is the buyer, staff, or vendor for this order.
	switch evaluateCustomerOrderAccess(actor, order) {
	case OrderAccessAllowed:
		// Authorized as buyer or platform staff
	case OrderAccessRedirectToVendor:
		http.Redirect(w, r, fmt.Sprintf("/vendor/orders/%d", order.ID), http.StatusSeeOther)
		return
	case OrderAccessDenied:
		// Unauthorized actor trying to view another organization's order
		h.redirectWithNotice(w, r, "/orders", "error", i18n.T(lang, "orders.order_not_found"))
		return
	}

	h.enrichOrderPricing(ctx, order)

	history, _ := h.commSvc.GetOrderHistory(ctx, id)

	var reviews []*org.Review
	if h.orgSvc != nil {
		reviews, _ = h.orgSvc.ListReviewsForOrder(ctx, id)
	}
	reviewedMap := make(map[int64]*org.Review)
	for _, rv := range reviews {
		if rv != nil {
			reviewedMap[rv.OrganizationID] = rv
		}
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("notice_msg")
	}

	h.renderPage(ctx, w, "render customer order detail page", pages.CustomerOrderDetail(order, history, noticeType, noticeMsg, lang, dir, reviewedMap))
}

// OrderAccessDecision represents the authorization outcome for customer order viewing.
type OrderAccessDecision int

const (
	OrderAccessAllowed OrderAccessDecision = iota
	OrderAccessRedirectToVendor
	OrderAccessDenied
)

// evaluateCustomerOrderAccess verifies whether the actor is the buyer, staff, or a vendor on the order.
func evaluateCustomerOrderAccess(actor authctx.Actor, order *commerce.Order) OrderAccessDecision {
	if order == nil {
		return OrderAccessDenied
	}
	isBuyer := order.CustomerID == actor.UserID || (order.OrganizationID != nil && actor.OrganizationID > 0 && *order.OrganizationID == actor.OrganizationID)
	if isBuyer || actor.IsStaff || actor.IsPlatformAdmin() {
		return OrderAccessAllowed
	}
	for _, sh := range order.Shipments {
		if sh != nil && actor.OrganizationID > 0 && sh.OrganizationID == actor.OrganizationID {
			return OrderAccessRedirectToVendor
		}
	}
	return OrderAccessDenied
}

