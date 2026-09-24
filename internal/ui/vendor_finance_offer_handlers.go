package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func parseIntDefault(v string, def int) int {
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// VendorEarningsOffersPage renders offers revenue and commissions for the vendor.
func (h *UIHandler) VendorEarningsOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/earnings/offers", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render vendor earnings offers", pages.VendorEarningsOffersPage(money.Zero, lang, dir))
}

// VendorOfferOrdersPage renders offer-based orders fulfilled by the vendor.
func (h *UIHandler) VendorOfferOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders/offers", http.StatusSeeOther)
		return
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var shipments []*commerce.OrderShipment
	var total int
	if h.commSvc != nil {
		if loaded, loaded2, err := h.commSvc.ListVendorShipmentsWithTotal(ctx, actor.OrganizationID, "", limit, offset); err != nil {
			h.logLoadErr(ctx, "commSvc.ListVendorShipmentsWithTotal", err)
		} else {
			shipments, total = loaded, loaded2
		}
	}

	h.renderPage(ctx, w, "render vendor offer orders", pages.VendorOfferOrdersPage(shipments, lang, dir, page, limit, total))
}

// VendorOfferOrderDetailPage renders single offer order details.
func (h *UIHandler) VendorOfferOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	shipID, _ := strconv.ParseInt(idStr, 10, 64)

	h.renderPage(ctx, w, "render vendor offer order detail", pages.VendorOfferOrderDetailPage(shipID, lang, dir))
}
