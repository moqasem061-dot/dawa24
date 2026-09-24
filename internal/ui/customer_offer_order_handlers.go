package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// CustomerOfferOrdersPage renders offer-based orders placed by the pharmacy.
func (h *UIHandler) CustomerOfferOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/orders/offers", http.StatusSeeOther)
		return
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var orders []*commerce.Order
	var total int
	if h.commSvc != nil {
		if loaded, loaded2, err := h.commSvc.ListCustomerOrdersWithTotal(ctx, actor.OrganizationID, limit, offset); err != nil {
			h.logLoadErr(ctx, "commSvc.ListCustomerOrdersWithTotal", err)
		} else {
			orders, total = loaded, loaded2
		}
	}

	h.renderPage(ctx, w, "render customer offer orders", pages.CustomerOfferOrdersPage(orders, lang, dir, page, limit, total))
}

// CustomerOfferOrderDetailPage renders single offer order details.
func (h *UIHandler) CustomerOfferOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/orders/%s", idStr), http.StatusSeeOther)
}

// CustomerOfferCheckoutPage renders dedicated checkout flow for a specific offer.
func (h *UIHandler) CustomerOfferCheckoutPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	offerID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || offerID <= 0 {
		http.Redirect(w, r, "/offers", http.StatusSeeOther)
		return
	}

	var offer *promo.Offer
	if h.promoSvc != nil {
		if loaded, err := h.promoSvc.GetOffer(database.AsSystem(ctx), offerID); err != nil {
			h.logLoadErr(ctx, "promoSvc.GetOffer", err)
		} else {
			offer = loaded
		}
	}

	if offer == nil {
		http.Redirect(w, r, "/offers", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render offer checkout", pages.CustomerOfferCheckoutPage(offer, lang, dir))
}

// CustomerAddOrderPage renders manual / quick order entry form.
func (h *UIHandler) CustomerAddOrderPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	h.renderPage(ctx, w, "render customer add order", pages.CustomerAddOrderPage(lang, dir))
}

// CustomerProductsMainAlias redirects /customer/products/main/{id} to /catalog/{id}.
func (h *UIHandler) CustomerProductsMainAlias(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/catalog/%s", idStr), http.StatusMovedPermanently)
}

// GuestOrderTrackingPage renders unauthenticated public order tracking screen.
func (h *UIHandler) GuestOrderTrackingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	orderNumber := r.URL.Query().Get("order_number")
	var order *commerce.Order
	if orderNumber != "" && h.commSvc != nil {
		if loaded, err := h.commSvc.GetOrderByNumber(database.AsSystem(ctx), orderNumber); err != nil {
			h.logLoadErr(ctx, "commSvc.GetOrderByNumber", err)
		} else {
			order = loaded
		}
	}

	h.renderPage(ctx, w, "render guest tracking", pages.GuestOrderTrackingPage(orderNumber, order, lang, dir))
}
