package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AdminAdApproveSubmit approves an ad from the admin UI.
func (h *UIHandler) AdminAdApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	ad, _ := h.promoSvc.GetAd(sysCtx, id)
	notes := r.PostFormValue("notes")
	if err := h.promoSvc.AdminApproveAd(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "error", h.safeMessage(err, langOf(r)))
		return
	}
	if ad != nil && ad.OrganizationID != nil && *ad.OrganizationID > 0 {
		adTitle := ad.TitleAr
		if adTitle == "" {
			adTitle = ad.Title
		}
		if adTitle == "" {
			adTitle = i18n.TDefault("w4_ui.s_81_81")
		}
		h.safeGo("notify-ad-status-approved", func() {
			h.notifyAdStatus(context.Background(), *ad.OrganizationID, adTitle, true, notes)
		})
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "success", i18n.T(langOf(r), "admin.promo.ad_approved_success"))
}

// AdminAdRejectSubmit rejects an ad from the admin UI.
func (h *UIHandler) AdminAdRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	ad, _ := h.promoSvc.GetAd(sysCtx, id)
	notes := r.PostFormValue("notes")
	if err := h.promoSvc.AdminRejectAd(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "error", h.safeMessage(err, langOf(r)))
		return
	}
	if ad != nil && ad.OrganizationID != nil && *ad.OrganizationID > 0 {
		adTitle := ad.TitleAr
		if adTitle == "" {
			adTitle = ad.Title
		}
		h.safeGo("notify-ad-status-rejected", func() {
			h.notifyAdStatus(context.Background(), *ad.OrganizationID, adTitle, false, notes)
		})
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "success", i18n.T(langOf(r), "admin.promo.ad_rejected_success"))
}

// AdminAdToggleSubmit toggles the active/inactive state of an ad.
func (h *UIHandler) AdminAdToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	updatedAd, err := h.promoSvc.AdminToggleAd(sysCtx, id)
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "error", h.safeMessage(err, lang))
		return
	}
	msg := "تم تفعيل الإعلان وظهوره على المنصة بنجاح."
	if !updatedAd.IsActive {
		msg = "تم إيقاف وتعطيل ظهور الإعلان مؤقتاً."
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "success", msg)
}

// AdminAdApproveEditSubmit approves a vendor-submitted edit request and applies changes.
func (h *UIHandler) AdminAdApproveEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	if err := h.promoSvc.ApproveAdEditRequest(sysCtx, id); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "success", "تم اعتماد وتطبيق تعديلات الإعلان بنجاح.")
}

// AdminAdRejectEditSubmit rejects a vendor-submitted edit request and keeps the live ad intact.
func (h *UIHandler) AdminAdRejectEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	sysCtx := database.AsSystem(ctx)
	if err := h.promoSvc.RejectAdEditRequest(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "success", "تم رفض طلب تعديل الإعلان، ويستمر الإعلان الأصلي بالعمل.")
}

// AdminOfferPackageCreateSubmit creates a new monetization / sponsorship package.
func (h *UIHandler) AdminOfferPackageCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.packages_service_unavailable"))
		return
	}

	nameAR := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEN := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAR == "" && nameEN == "" {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.package_name_required"))
		return
	}
	if nameAR == "" {
		nameAR = nameEN
	}
	if nameEN == "" {
		nameEN = nameAR
	}

	descAR := strings.TrimSpace(r.PostFormValue("desc_ar"))
	descEN := strings.TrimSpace(r.PostFormValue("desc_en"))

	priceStr := strings.TrimSpace(r.PostFormValue("price"))
	price, err := money.Parse(priceStr)
	if err != nil {
		price = money.Zero
	}

	durationDays, _ := strconv.Atoi(r.PostFormValue("duration_days"))
	if durationDays <= 0 {
		durationDays = 30
	}

	credits, _ := strconv.Atoi(r.PostFormValue("credits"))
	if credits <= 0 {
		credits = 10
	}

	maxOffers, _ := strconv.Atoi(r.PostFormValue("max_offers"))
	if maxOffers <= 0 {
		maxOffers = credits * 2
	}

	tierLevel, _ := strconv.Atoi(r.PostFormValue("tier_level"))
	if tierLevel <= 0 || tierLevel > 5 {
		tierLevel = 1
	}

	sortOrder, _ := strconv.Atoi(r.PostFormValue("sort_order"))
	badgeColor := strings.TrimSpace(r.PostFormValue("badge_color"))
	if badgeColor == "" {
		badgeColor = "#0284c7"
	}

	isFeatured := r.PostFormValue("is_featured") == "true"
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on"

	pkg := &promo.OfferPackage{
		Name:         i18n.New(nameAR, nameEN),
		Description:  i18n.New(descAR, descEN),
		Price:        price,
		DurationDays: durationDays,
		Credits:      credits,
		MaxOffers:    maxOffers,
		TierLevel:    tierLevel,
		SortOrder:    sortOrder,
		BadgeColor:   badgeColor,
		IsFeatured:   isFeatured,
		IsActive:     isActive,
	}

	sysCtx := database.AsSystem(ctx)
	if _, err := h.promoSvc.AdminCreatePackage(sysCtx, pkg); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "success", i18n.T(lang, "admin.promo.package_created_success"))
}

// AdminOfferPackageEditSubmit updates an offer package.
func (h *UIHandler) AdminOfferPackageEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.packages_service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.package_invalid_id"))
		return
	}

	nameAR := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEN := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAR == "" && nameEN == "" {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.package_name_required"))
		return
	}
	if nameAR == "" {
		nameAR = nameEN
	}
	if nameEN == "" {
		nameEN = nameAR
	}

	descAR := strings.TrimSpace(r.PostFormValue("desc_ar"))
	descEN := strings.TrimSpace(r.PostFormValue("desc_en"))

	priceStr := strings.TrimSpace(r.PostFormValue("price"))
	price, err := money.Parse(priceStr)
	if err != nil {
		price = money.Zero
	}

	durationDays, _ := strconv.Atoi(r.PostFormValue("duration_days"))
	if durationDays <= 0 {
		durationDays = 30
	}

	credits, _ := strconv.Atoi(r.PostFormValue("credits"))
	if credits <= 0 {
		credits = 10
	}

	maxOffers, _ := strconv.Atoi(r.PostFormValue("max_offers"))
	if maxOffers <= 0 {
		maxOffers = credits * 2
	}

	tierLevel, _ := strconv.Atoi(r.PostFormValue("tier_level"))
	if tierLevel <= 0 || tierLevel > 5 {
		tierLevel = 1
	}

	sortOrder, _ := strconv.Atoi(r.PostFormValue("sort_order"))
	badgeColor := strings.TrimSpace(r.PostFormValue("badge_color"))
	if badgeColor == "" {
		badgeColor = "#0284c7"
	}

	isFeatured := r.PostFormValue("is_featured") == "true"
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on"

	pkg := &promo.OfferPackage{
		ID:           id,
		Name:         i18n.New(nameAR, nameEN),
		Description:  i18n.New(descAR, descEN),
		Price:        price,
		DurationDays: durationDays,
		Credits:      credits,
		MaxOffers:    maxOffers,
		TierLevel:    tierLevel,
		SortOrder:    sortOrder,
		BadgeColor:   badgeColor,
		IsFeatured:   isFeatured,
		IsActive:     isActive,
	}

	sysCtx := database.AsSystem(ctx)
	if _, err := h.promoSvc.AdminUpdatePackage(sysCtx, pkg); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "success", i18n.T(lang, "admin.promo.package_updated_success"))
}

// AdminOfferPackageToggleSubmit toggles active status for an offer package.
func (h *UIHandler) AdminOfferPackageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.packages_service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.package_invalid_id"))
		return
	}

	active := r.PostFormValue("active") == "true" || r.PostFormValue("active") == "1" || r.PostFormValue("active") == "on"
	sysCtx := database.AsSystem(ctx)
	if err := h.promoSvc.AdminTogglePackageActive(sysCtx, id, active); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", h.safeMessage(err, lang))
		return
	}

	msg := i18n.T(lang, "admin.promo.package_deactivated_success")
	if active {
		msg = i18n.T(lang, "admin.promo.package_activated_success")
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "success", msg)
}
