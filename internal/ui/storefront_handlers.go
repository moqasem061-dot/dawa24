package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorStorefrontPage renders the supplier's featured sections manager.
func (h *UIHandler) VendorStorefrontPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/storefront", http.StatusSeeOther)
		return
	}

	var sections []*promo.HighlightSection
	if h.promoSvc != nil {
		sections, _ = h.promoSvc.ListHighlightSectionsByOrg(ctx, actor.OrganizationID)
	}

	h.renderPage(ctx, w, "render vendor storefront", pages.VendorStorefront(lang, dir, sections))
}

// VendorStorefrontSectionSubmit creates a supplier featured section.
func (h *UIHandler) VendorStorefrontSectionSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/storefront", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", i18n.T(lang, "admin.import.service_unavailable"))
		return
	}

	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", i18n.T(lang, "vendor.storefront.title_ar_required"))
		return
	}

	descAr := strings.TrimSpace(r.PostFormValue("description_ar"))
	descEn := strings.TrimSpace(r.PostFormValue("description_en"))
	secType := strings.TrimSpace(r.PostFormValue("section_type"))
	if secType == "" {
		secType = "about"
	}
	color := strings.TrimSpace(r.PostFormValue("color"))
	if color == "" {
		color = "#0284c7"
	}
	slug := strings.TrimSpace(r.PostFormValue("slug"))
	order, _ := strconv.Atoi(r.PostFormValue("display_order"))
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on" || r.PostFormValue("is_active") == "1"
	showInHeader := r.PostFormValue("show_in_header") == "true" || r.PostFormValue("show_in_header") == "on" || r.PostFormValue("show_in_header") == "1" || r.PostFormValue("show_in_header") == ""

	title := i18n.New(titleAr, titleEn)
	description := i18n.New(descAr, descEn)

	if _, err := h.promoSvc.CreateFeaturedSection(ctx, actor.OrganizationID, title, description, secType, color, slug, order, isActive, showInHeader); err != nil {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/storefront", "success", i18n.T(lang, "vendor.storefront.section_created_success"))
}

// VendorStorefrontSectionUpdateSubmit updates an existing featured section.
func (h *UIHandler) VendorStorefrontSectionUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/storefront", http.StatusSeeOther)
		return
	}

	sectionID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if sectionID <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", i18n.T(lang, "vendor.storefront.section_edit_failed"))
		return
	}

	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", i18n.T(lang, "vendor.storefront.title_ar_required"))
		return
	}

	descAr := strings.TrimSpace(r.PostFormValue("description_ar"))
	descEn := strings.TrimSpace(r.PostFormValue("description_en"))
	secType := strings.TrimSpace(r.PostFormValue("section_type"))
	if secType == "" {
		secType = "about"
	}
	color := strings.TrimSpace(r.PostFormValue("color"))
	if color == "" {
		color = "#0284c7"
	}
	slug := strings.TrimSpace(r.PostFormValue("slug"))
	order, _ := strconv.Atoi(r.PostFormValue("display_order"))
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on" || r.PostFormValue("is_active") == "1"
	showInHeader := r.PostFormValue("show_in_header") == "true" || r.PostFormValue("show_in_header") == "on" || r.PostFormValue("show_in_header") == "1"

	orgID := actor.OrganizationID
	sec := &promo.HighlightSection{
		ID:             sectionID,
		OwnerType:      "organization",
		OrganizationID: &orgID,
		Title:          i18n.New(titleAr, titleEn),
		Description:    i18n.New(descAr, descEn),
		SectionType:    secType,
		Color:          color,
		Slug:           slug,
		DisplayOrder:   order,
		IsActive:       isActive,
		ShowInHeader:   showInHeader,
	}

	if err := h.promoSvc.UpdateFeaturedSection(ctx, sec); err != nil {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/storefront", "success", i18n.T(lang, "vendor.storefront.section_updated_success"))
}

// VendorStorefrontSectionDeleteSubmit deletes a featured section.
func (h *UIHandler) VendorStorefrontSectionDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/storefront", http.StatusSeeOther)
		return
	}

	sectionID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if sectionID <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", i18n.T(lang, "vendor.storefront.section_delete_failed"))
		return
	}

	if err := h.promoSvc.DeleteFeaturedSection(ctx, sectionID, actor.OrganizationID); err != nil {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/storefront", "success", i18n.T(lang, "vendor.storefront.section_deleted_success"))
}

// VendorStorefrontSectionToggleSubmit toggles the active state of a section.
func (h *UIHandler) VendorStorefrontSectionToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/storefront", http.StatusSeeOther)
		return
	}

	sectionID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if sectionID <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", i18n.T(lang, "vendor.storefront.section_toggle_failed"))
		return
	}

	sec, err := h.promoSvc.GetFeaturedSection(ctx, sectionID)
	if err != nil || sec == nil || sec.OrganizationID == nil || *sec.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", i18n.T(lang, "vendor.storefront.section_not_found_or_unauthorized"))
		return
	}

	sec.IsActive = !sec.IsActive
	if err := h.promoSvc.UpdateFeaturedSection(ctx, sec); err != nil {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/storefront", "success", i18n.T(lang, "vendor.storefront.section_status_updated"))
}
