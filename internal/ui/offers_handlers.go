package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// OffersPage renders the public offers listing.
func (h *UIHandler) OffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !features.Enabled(ctx, "offers.enabled") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)

	view := pages.OffersView{
		Sort:     offersSort(r.URL.Query().Get("sort")),
		Page:     pagination.PageNumber(r),
		PageSize: offersPageSize(r),
	}
	actor, _ := authctx.From(ctx)
	buyer := h.buyerOfferQuery(ctx, actor, 0)
	view.Notice = buyer.Notice

	var cards []*pages.OfferCardData
	if h.promoSvc != nil && buyer.Notice == "" {
		q := buyer.Query
		q.Sort = promo.BuyerOfferSort(view.Sort)
		q.Limit, q.Offset = view.PageSize, (view.Page-1)*view.PageSize
		offers, total, err := h.promoSvc.ListBuyerOffers(ctx, q)
		if err != nil {
			h.log.ErrorContext(ctx, "offers page: list buyer offers", "error", err)
		}
		cards, view.Total = offerCards(offers, lang, q.Buying), total
	}

	h.renderPage(ctx, w, "render offers page", pages.OffersPage(lang, dir, cards, view))
}

// offersSort accepts only the orders the board offers.
func offersSort(raw string) string {
	switch s := promo.BuyerOfferSort(strings.TrimSpace(raw)); s {
	case promo.SortOffersDiscountDesc, promo.SortOffersDiscountAsc, promo.SortOffersPriceAsc, promo.SortOffersPriceDesc:
		return string(s)
	}
	return string(promo.SortOffersNewest)
}

// offersPageSize is the card grid's page size: one of the grid sizes.
func offersPageSize(r *http.Request) int {
	switch n, _ := strconv.Atoi(r.URL.Query().Get("limit")); n {
	case 12, 24, 48, 96:
		return n
	}
	return 24
}

// OfferDetailPage renders one offer with its full products and records an impression.
func (h *UIHandler) OfferDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.renderError(w, r, apperr.NotFound("offer"))
		return
	}
	if h.promoSvc == nil {
		h.renderError(w, r, apperr.NotFound("offer"))
		return
	}

	sp, err := h.promoSvc.GetSpecialOffer(ctx, id)
	if err != nil || sp == nil {
		// Fallback to GetOffer if not structured as special offer
		o, oErr := h.promoSvc.GetOffer(ctx, id)
		if oErr != nil || o == nil {
			h.renderError(w, r, err)
			return
		}
		starts := o.StartsAt
		expires := o.ExpiresAt
		sp = &promo.SpecialOffer{
			ID:                 o.ID,
			PublicID:           o.PublicID,
			OrganizationID:     o.OrganizationID,
			Title:              o.Title,
			Description:        o.Description,
			DiscountPercentage: float64(o.DiscountValue.Minor()) / 100.0,
			MinOrderAmount:     o.MinOrderAmount,
			StartDate:          &starts,
			EndDate:            &expires,
			Status:             "active",
			AdminStatus:        o.AdminStatus,
		}
	}

	// A supplier who reaches their own promotion by id is sent back to the
	// board rather than shown a buy button for their own stock. Recording a
	// view first would also let them inflate their own offer's statistics.
	if ownedByBuyer(buyerOrgID(ctx), sp.OrganizationID) {
		h.redirectWithNotice(w, r, "/offers", "error", i18n.T(lang, "err.own_organization_supply"))
		return
	}

	// The offer rule decides, for a visitor as for a buyer: a withdrawn,
	// unapproved or expired offer, an unapproved supplier or a deleted supplier
	// branch sends everyone back to the board with the reason.
	actor, _ := authctx.From(ctx)
	buyer := h.buyerOfferQuery(ctx, actor, 0)
	q := buyer.Query
	q.OfferID = id
	verdict, err := h.promoSvc.OfferVerdict(ctx, q)
	if err != nil {
		h.log.ErrorContext(ctx, "offer detail: verdict", "offer_id", id, "error", err)
		h.redirectWithNotice(w, r, "/offers", "error", offerCheckFailed)
		return
	}
	if reason := verdict.Reason(false); reason != promo.OfferOK {
		msg := validateSpecialOfferForCheckout(sp)
		if msg == "" {
			msg = offerRefusal(reason)
		}
		h.redirectWithNotice(w, r, "/offers", "error", msg)
		return
	}

	locs, _ := h.promoSvc.ListSpecialOfferLocations(ctx, id)
	sp.Locations = locs

	isBuyer := q.Buying
	isCovered, covReason := true, ""
	if isBuyer {
		isCovered, covReason = false, buyer.Notice
		if buyer.Notice == "" {
			covReason = offerRefusal(verdict.Reason(true))
			isCovered = covReason == ""
		}
	}
	var customerBranch *org.Branch
	if isBuyer {
		customerBranch = h.buyingBranch(ctx, &actor)
	}

	_ = h.promoSvc.RecordOfferView(ctx, id)

	var orgInfo *org.Organization
	if h.orgSvc != nil && sp.OrganizationID > 0 {
		if o, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), sp.OrganizationID); err == nil && o != nil {
			orgInfo = o
			if sp.OrganizationName == "" {
				if o.LegalName != "" {
					sp.OrganizationName = o.LegalName
				} else if !o.TradeName.IsEmpty() {
					sp.OrganizationName = o.TradeName.Get(i18n.Lang(lang))
				}
			}
		}
	}

	data := pages.OfferDetailPageData{
		Offer:          sp,
		Organization:   orgInfo,
		Products:       sp.Products,
		Locations:      locs,
		IsCustomerUser: isBuyer,
		IsCovered:      isCovered,
		CoverageReason: covReason,
		CustomerBranch: customerBranch,
	}

	h.renderPage(ctx, w, "render offer detail", pages.OfferDetail(lang, dir, data))
}

// OfferClickSubmit records an offer click and sends the user to the catalogue.
func (h *UIHandler) OfferClickSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.promoSvc != nil {
		if id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64); err == nil {
			_ = h.promoSvc.RecordOfferClick(ctx, id)
		}
	}
	http.Redirect(w, r, "/catalog", http.StatusSeeOther)
}
