package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// VendorOffersPackagesPage renders available packages and current purchases for vendor.
func (h *UIHandler) VendorOffersPackagesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers-packages", http.StatusSeeOther)
		return
	}
	ctx = database.WithTenant(ctx, actor.OrganizationID)

	var packages []*promo.OfferPackage
	var purchases []*promo.SponsorshipPurchase
	if h.promoSvc != nil {
		if loaded, err := h.promoSvc.ListPackages(ctx); err != nil {
			h.logLoadErr(ctx, "promoSvc.ListPackages", err)
		} else {
			packages = loaded
		}
		if loaded, err := h.promoSvc.ListSponsorshipPurchases(ctx); err != nil {
			h.logLoadErr(ctx, "promoSvc.ListSponsorshipPurchases", err)
		} else {
			purchases = loaded
		}
	}

	var walletBal money.Amount
	if h.billSvc != nil {
		sysCtx := database.AsSystem(ctx)
		if w, err := h.billSvc.GetWallet(sysCtx, actor.UserID, "EGP"); err == nil && w != nil {
			walletBal = w.Available()
		}
	}

	data := pages.SponsorshipRequestsData{
		Packages:      packages,
		Purchases:     purchases,
		OrgID:         actor.OrganizationID,
		WalletBalance: walletBal,
		NoticeType:    r.URL.Query().Get("notice_type"),
		NoticeMsg:     r.URL.Query().Get("notice"),
	}

	h.renderPage(ctx, w, "render vendor offers packages", pages.VendorOffersPackagesPageWithData(lang, dir, data))
}

// VendorOffersPackagesSponsorshipsPage renders vendor's sponsored offers list.
func (h *UIHandler) VendorOffersPackagesSponsorshipsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	h.renderPage(ctx, w, "render vendor offers packages sponsorships", pages.VendorOffersPackagesSponsorshipsPage(lang, dir))
}

// VendorOffersPackagesPromotionsPage renders vendor's promotional campaigns.
func (h *UIHandler) VendorOffersPackagesPromotionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	h.renderPage(ctx, w, "render vendor offers packages promotions", pages.VendorOffersPackagesPromotionsPage(lang, dir))
}

// VendorAdsPage renders vendor's active banners and ads with statistics and creation wizard.
func (h *UIHandler) VendorAdsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ads", http.StatusSeeOther)
		return
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var ads []*promo.Ad
	var totalAds int
	var activePurchases []*promo.SponsorshipPurchase
	if h.promoSvc != nil {
		if loaded, loaded2, err := h.promoSvc.ListAdsByOrgWithTotal(ctx, limit, offset); err != nil {
			h.logLoadErr(ctx, "promoSvc.ListAdsByOrgWithTotal", err)
		} else {
			ads, totalAds = loaded, loaded2
		}
		if loaded, err := h.promoSvc.ListActiveSponsorshipPurchases(ctx); err != nil {
			h.logLoadErr(ctx, "promoSvc.ListActiveSponsorshipPurchases", err)
		} else {
			activePurchases = loaded
		}
	}

	totalCredits := 0
	for _, p := range activePurchases {
		if p != nil {
			totalCredits += p.CreditsRemainingInt()
		}
	}

	itemOptions := h.loadVendorInStockItems(ctx, actor.OrganizationID)

	noticeType := r.URL.Query().Get("notice")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	noticeMsg := r.URL.Query().Get("msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("message")
	}
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("notice_msg")
	}

	data := pages.VendorAdsData{
		Ads:             ads,
		ItemOptions:     itemOptions,
		ActivePurchases: activePurchases,
		TotalCredits:    totalCredits,
		NoticeType:      noticeType,
		NoticeMsg:       noticeMsg,
		Page:            page,
		PerPage:         limit,
		TotalCount:      totalAds,
	}

	h.renderPage(ctx, w, "render vendor ads", pages.VendorAdsPage(lang, dir, data))
}

// VendorOffersLocationsPage renders vendor's offer geographic coverage.
func (h *UIHandler) VendorOffersLocationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	h.renderPage(ctx, w, "render vendor offers locations", pages.VendorOffersLocationsPage(lang, dir))
}

// PublicPromotionTrackClick records a click to promo.offer_clicks with open redirect guard.
func (h *UIHandler) PublicPromotionTrackClick(w http.ResponseWriter, r *http.Request) {
	offerIDStr := chi.URLParam(r, "offer")
	offerID, _ := strconv.ParseInt(offerIDStr, 10, 64)

	h.log.InfoContext(r.Context(), "tracked promotion click", "offer_id", offerID)
	// Safe redirect strictly to internal offer detail
	http.Redirect(w, r, fmt.Sprintf("/offers/%d", offerID), http.StatusSeeOther)
}
