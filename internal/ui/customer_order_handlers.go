package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
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

	order, err := h.commSvc.GetOrder(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
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

// CustomerOrderEditSubmit handles customer edits to quantities and items of a pending order.
