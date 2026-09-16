package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminAdvProductsPage renders the product sponsorships and promoted items dashboard.
func (h *UIHandler) AdminAdvProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	tab := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tab")))
	if tab == "" {
		tab = "all"
	}
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	orgFilterID, _ := strconv.ParseInt(r.URL.Query().Get("org_id"), 10, 64)
	packageFilterID, _ := strconv.ParseInt(r.URL.Query().Get("package_id"), 10, 64)
	tierLevel, _ := strconv.Atoi(r.URL.Query().Get("tier_level"))
	dateFromStr := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateToStr := strings.TrimSpace(r.URL.Query().Get("date_to"))

	var startsFrom, startsTo *time.Time
	if t, err := time.Parse("2006-01-02", dateFromStr); err == nil {
		startsFrom = &t
	}
	if t, err := time.Parse("2006-01-02", dateToStr); err == nil {
		// Include the full ending day
		inclusive := t.Add(24 * time.Hour)
		startsTo = &inclusive
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	filter := promo.AdminSponsorshipFilter{
		Tab:            tab,
		Search:         searchQuery,
		OrganizationID: orgFilterID,
		PackageID:      packageFilterID,
		TierLevel:      tierLevel,
		StartsFrom:     startsFrom,
		StartsTo:       startsTo,
		Limit:          limit,
		Offset:         offset,
	}

	var rows []*promo.AdminSponsorshipRow
	var total int
	var counts promo.AdminSponsorshipCounts

	if h.promoSvc != nil {
		if rws, tot, err := h.promoSvc.ListAdminSponsorshipRows(sysCtx, filter); err == nil {
			rows = rws
			total = tot
		}
		if c, err := h.promoSvc.AdminSponsorshipCounts(sysCtx, filter); err == nil {
			counts = c
		}
	}

	var packages []*promo.OfferPackage
	if h.promoSvc != nil {
		if pkgs, err := h.promoSvc.AdminListPackages(sysCtx); err == nil {
			packages = pkgs
		}
	}

	var vendors []*org.Organization
	if h.orgSvc != nil {
		vendorType := org.TypeVendor
		if orgs, err := h.orgSvc.ListOrganizations(sysCtx, &vendorType, nil, 100, 0); err == nil {
			vendors = orgs
		}
	}

	var catalogProducts []*catalog.Product
	if h.catSvc != nil {
		if prods, err := h.catSvc.ListProducts(sysCtx, string(catalog.StatusActive), 100, 0); err == nil {
			catalogProducts = prods
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

	data := pages.AdminAdvProductsData{
		Rows:              rows,
		TotalCount:        counts.Total,
		ActiveCount:       counts.Active,
		PendingCount:      counts.Pending,
		ExpiredCount:      counts.Expired,
		RejectedCount:     counts.Rejected,
		Packages:          packages,
		Vendors:           vendors,
		CatalogProducts:   catalogProducts,
		ActiveTab:         tab,
		SearchQuery:       searchQuery,
		SelectedOrgID:     orgFilterID,
		SelectedPackageID: packageFilterID,
		SelectedTierLevel: tierLevel,
		DateFrom:          dateFromStr,
		DateTo:            dateToStr,
		NoticeType:        noticeType,
		NoticeMsg:         noticeMsg,
		Page:              page,
		PerPage:           limit,
		FilteredTotal:     total,
	}

	h.renderPage(ctx, w, "render admin adv-products page", pages.AdminAdvProductsPage(lang, dir, data))
}

// AdminAdvProductApproveSubmit approves and activates a product sponsorship request.
func (h *UIHandler) AdminAdvProductApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "promo.sponsorship.invalid_id"))
		return
	}

	sysCtx := database.AsSystem(ctx)
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	req, err := h.promoSvc.AdminApproveSponsorshipRequest(sysCtx, id, notes)
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", h.safeMessage(err, lang))
		return
	}

	if req != nil && req.OrganizationID > 0 {
		pkgName := i18n.T(lang, "promo.credits.statement_title")
		if req.Package != nil {
			pkgName = req.Package.Name.Get(i18n.ParseLang(lang))
		}
		h.safeGo("notify-sponsorship-approved", func() {
			h.notifySponsorshipStatus(context.Background(), req.OrganizationID, pkgName, true, notes)
		})
	}

	h.redirectWithNotice(w, r, "/admin/adv-products", "success", i18n.T(lang, "promo.sponsorship.approved_success"))
}

// AdminAdvProductRejectSubmit rejects a product sponsorship request and refunds credits.
func (h *UIHandler) AdminAdvProductRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "promo.sponsorship.invalid_id"))
		return
	}

	sysCtx := database.AsSystem(ctx)
	req, _ := h.promoSvc.GetSponsorshipRequestByID(sysCtx, id)
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	if notes == "" {
		notes = i18n.T(lang, "promo.sponsorship.default_reject_notes")
	}

	if err := h.promoSvc.AdminRejectSponsorshipRequest(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", h.safeMessage(err, lang))
		return
	}

	if req != nil && req.OrganizationID > 0 {
		pkgName := i18n.T(lang, "promo.credits.statement_title")
		if req.Package != nil {
			pkgName = req.Package.Name.Get(i18n.ParseLang(lang))
		}
		h.safeGo("notify-sponsorship-rejected", func() {
			h.notifySponsorshipStatus(context.Background(), req.OrganizationID, pkgName, false, notes)
		})
	}

	if req != nil && req.AdminStatus == "approved" {
		h.redirectWithNotice(w, r, "/admin/adv-products", "success", "تم إلغاء الرعاية بنجاح واسترداد الرصيد إلى محفظة المورد")
	} else {
		h.redirectWithNotice(w, r, "/admin/adv-products", "success", i18n.T(lang, "promo.sponsorship.rejected_success"))
	}
}

// AdminAdvProductCreateSubmit creates an instant product sponsorship directly from the admin panel.
func (h *UIHandler) AdminAdvProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	_ = r.ParseForm()
	orgID, _ := strconv.ParseInt(r.PostFormValue("org_id"), 10, 64)
	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	packageID, _ := strconv.ParseInt(r.PostFormValue("package_id"), 10, 64)
	days, _ := strconv.Atoi(r.PostFormValue("duration_days"))

	if orgID <= 0 || productID <= 0 || packageID <= 0 {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "promo.sponsorship.required_fields"))
		return
	}

	if days <= 0 {
		days = 30
	}

	sysCtx := database.AsSystem(ctx)
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(days) * 24 * time.Hour)

	sr := &promo.SponsorshipRequest{
		OrganizationID: orgID,
		PackageID:      packageID,
		ItemType:       promo.SponsorItemProduct,
		ItemID:         productID,
		CreditsUsed:    1,
		AdminStatus:    promo.AdminApproved,
		Status:         promo.SRSActive,
		StartsAt:       now,
		ExpiresAt:      expiresAt,
	}

	if err := h.promoSvc.AdminCreateDirectSponsorship(sysCtx, sr); err != nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/adv-products", "success", i18n.T(lang, "promo.sponsorship.created_success"))
}
