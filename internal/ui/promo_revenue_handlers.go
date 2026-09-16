package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOffersPackagesHubPage renders the monetization and packages overview hub.
func (h *UIHandler) AdminOffersPackagesHubPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "packages"
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var packages []*promo.OfferPackage
	var requests []*promo.SponsorshipRequest
	var ads []*promo.Ad

	var totalPkgs, activePkgs int
	var totalRequests, pendingRequests, approvedRequests int
	var totalAds, pendingAds int

	if h.promoSvc != nil {
		if pkgs, err := h.promoSvc.AdminListPackages(ctx); err == nil {
			packages = pkgs
			totalPkgs = len(pkgs)
			for _, p := range pkgs {
				if p != nil && p.IsActive {
					activePkgs++
				}
			}
		}

		if reqs, total, err := h.promoSvc.AdminListSponsorshipRequestsWithTotal(database.AsSystem(ctx), limit, offset); err == nil {
			requests = reqs
			totalRequests = total
			for _, req := range reqs {
				if req != nil {
					if req.AdminStatus == "pending" {
						pendingRequests++
					} else if req.AdminStatus == "approved" {
						approvedRequests++
					}
				}
			}
		}

		if adList, total, err := h.promoSvc.AdminListAdsWithTotal(database.AsSystem(ctx), limit, offset); err == nil {
			ads = adList
			totalAds = total
			for _, a := range adList {
				if a != nil && a.AdminStatus == "pending" {
					pendingAds++
				}
			}
		}
	}

	noticeType := r.URL.Query().Get("notice")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	noticeMsg := r.URL.Query().Get("msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("message")
	}

	data := pages.AdminOffersPackagesData{
		Packages:         packages,
		Requests:         requests,
		Ads:              ads,
		TotalPackages:    totalPkgs,
		ActivePackages:   activePkgs,
		TotalRequests:    totalRequests,
		PendingRequests:  pendingRequests,
		ApprovedRequests: approvedRequests,
		TotalAds:         totalAds,
		PendingAds:       pendingAds,
		ActiveTab:        tab,
		NoticeType:       noticeType,
		NoticeMsg:        noticeMsg,
		ReqPage:          page,
		ReqPerPage:       limit,
		AdsPage:          page,
		AdsPerPage:       limit,
	}

	h.renderPage(ctx, w, "render admin offers packages hub", pages.AdminOffersPackagesHubPage(lang, dir, data))
}

// AdminOfferPackagesListPage renders list of offer packages and pricing tiers.
func (h *UIHandler) AdminOfferPackagesListPage(w http.ResponseWriter, r *http.Request) {
	h.AdminOffersPackagesHubPage(w, r)
}

// AdminOfferPackageDetailPage renders single offer package details and features.
func (h *UIHandler) AdminOfferPackageDetailPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/offers-packages?tab=packages", http.StatusMovedPermanently)
}

// AdminOfferSponsorshipsPage renders list of sponsored offers and pending requests.
func (h *UIHandler) AdminOfferSponsorshipsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var requests []*promo.SponsorshipRequest
	var total int
	if h.promoSvc != nil {
		requests, total, _ = h.promoSvc.AdminListSponsorshipRequestsWithTotal(database.AsSystem(ctx), limit, offset)
	}

	h.renderPage(ctx, w, "render admin offer sponsorships", pages.AdminOfferSponsorshipsPage(lang, dir, pages.AdminOfferSponsorshipsPageData{
		Requests:   requests,
		Page:       page,
		PerPage:    limit,
		TotalCount: total,
	}))
}

// AdminOfferPromotionsPage renders promotional campaigns list.
func (h *UIHandler) AdminOfferPromotionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin offer promotions", pages.AdminOfferPromotionsPage(lang, dir))
}

// AdminAdsListPage renders advertisements and banners management table with approval actions.
func (h *UIHandler) AdminAdsListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var ads []*promo.Ad
	var total int
	if h.promoSvc != nil {
		ads, total, _ = h.promoSvc.AdminListAdsWithTotal(database.AsSystem(ctx), limit, offset)
	}

	h.renderPage(ctx, w, "render admin ads list", pages.AdminAdsListPage(lang, dir, pages.AdminAdsListPageData{
		Ads:        ads,
		Page:       page,
		PerPage:    limit,
		TotalCount: total,
	}))
}

// AdminAdPlansPage renders advertising placement plans (unifies into /admin/offers-packages).
func (h *UIHandler) AdminAdPlansPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/offers-packages?tab=packages", http.StatusMovedPermanently)
}

// AdminOfferAnalyticsViewsPage renders views time-series and aggregate report.
func (h *UIHandler) AdminOfferAnalyticsViewsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin offer analytics views", pages.AdminOfferAnalyticsViewsPage(lang, dir))
}

// AdminOfferAnalyticsClicksPage renders clicks time-series and CTR report.
func (h *UIHandler) AdminOfferAnalyticsClicksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin offer analytics clicks", pages.AdminOfferAnalyticsClicksPage(lang, dir))
}

// AdminOfferLocationsPage renders geographic coverage distribution for offers.
func (h *UIHandler) AdminOfferLocationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var offerID int64
	if idParam := chi.URLParam(r, "id"); idParam != "" {
		offerID, _ = strconv.ParseInt(idParam, 10, 64)
	}
	if offerID == 0 {
		if idQuery := r.URL.Query().Get("offer_id"); idQuery != "" {
			offerID, _ = strconv.ParseInt(idQuery, 10, 64)
		}
	}

	filter := promo.OfferLocationsFilter{
		OfferID:     offerID,
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
		AdminStatus: strings.TrimSpace(r.URL.Query().Get("admin_status")),
		Governorate: strings.TrimSpace(r.URL.Query().Get("governorate")),
		Search:      strings.TrimSpace(r.URL.Query().Get("q")),
		Limit:       limit,
		Offset:      offset,
	}

	var locations []*promo.OfferLocationAdminRow
	var stats promo.OfferLocationsStats
	var total int

	if h.promoSvc != nil {
		locations, stats, total, _ = h.promoSvc.ListAdminOfferLocations(database.AsSystem(ctx), filter)
	}

	data := pages.AdminOfferLocationsData{
		Locations:  locations,
		Stats:      stats,
		Filter:     filter,
		Page:       page,
		PerPage:    limit,
		TotalCount: total,
	}

	h.renderPage(ctx, w, "render admin offer locations", pages.AdminOfferLocationsPage(lang, dir, data))
}

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
		packages, _ = h.promoSvc.ListPackages(ctx)
		purchases, _ = h.promoSvc.ListSponsorshipPurchases(ctx)
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
		ads, totalAds, _ = h.promoSvc.ListAdsByOrgWithTotal(ctx, limit, offset)
		activePurchases, _ = h.promoSvc.ListActiveSponsorshipPurchases(ctx)
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
