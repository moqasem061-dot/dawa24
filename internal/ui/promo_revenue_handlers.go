package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
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
		if pkgs, err := h.promoSvc.AdminListPackages(ctx); err != nil {
			h.logLoadErr(ctx, "promoSvc.AdminListPackages", err)
		} else {
			packages = pkgs
			totalPkgs = len(pkgs)
			for _, p := range pkgs {
				if p != nil && p.IsActive {
					activePkgs++
				}
			}
		}

		if reqs, total, err := h.promoSvc.AdminListSponsorshipRequestsWithTotal(database.AsSystem(ctx), limit, offset); err != nil {
			h.logLoadErr(ctx, "promoSvc.AdminListSponsorshipRequestsWithTotal", err)
		} else {
			requests = reqs
			totalRequests = total
			for _, req := range reqs {
				if req != nil {
					switch req.AdminStatus {
					case "pending":
						pendingRequests++
					case "approved":
						approvedRequests++
					}
				}
			}
		}

		if adList, total, err := h.promoSvc.AdminListAdsWithTotal(database.AsSystem(ctx), limit, offset); err != nil {
			h.logLoadErr(ctx, "promoSvc.AdminListAdsWithTotal", err)
		} else {
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
		if loaded, loaded2, err := h.promoSvc.AdminListSponsorshipRequestsWithTotal(database.AsSystem(ctx), limit, offset); err != nil {
			h.logLoadErr(ctx, "promoSvc.AdminListSponsorshipRequestsWithTotal", err)
		} else {
			requests, total = loaded, loaded2
		}
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
		if loaded, loaded2, err := h.promoSvc.AdminListAdsWithTotal(database.AsSystem(ctx), limit, offset); err != nil {
			h.logLoadErr(ctx, "promoSvc.AdminListAdsWithTotal", err)
		} else {
			ads, total = loaded, loaded2
		}
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
		if loaded, loaded2, loaded3, err := h.promoSvc.ListAdminOfferLocations(database.AsSystem(ctx), filter); err != nil {
			h.logLoadErr(ctx, "promoSvc.ListAdminOfferLocations", err)
		} else {
			locations, stats, total = loaded, loaded2, loaded3
		}
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
