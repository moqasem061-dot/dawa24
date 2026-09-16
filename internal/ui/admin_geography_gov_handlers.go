package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AdminGovernorateCreateSubmit adds a new main governorate.
func (h *UIHandler) AdminGovernorateCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	_ = r.ParseForm()
	nameAr := strings.TrimSpace(r.PostFormValue("gov_name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("gov_name_en"))
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.gov_name_ar_required"))
		return
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("gov_lat"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("gov_lon"), 64)

	govRadiusMeters, _ := strconv.Atoi(r.PostFormValue("gov_coverage_radius_meters"))
	if govRadiusMeters <= 0 {
		govRadiusMeters, _ = strconv.Atoi(r.PostFormValue("coverage_radius_meters"))
	}
	if govRadiusMeters <= 0 {
		govRadiusMeters, _ = strconv.Atoi(r.PostFormValue("radius"))
	}
	if govRadiusMeters <= 0 {
		govRadiusMeters = 25000
	}

	gov := &platformadmin.Governorate{
		CountryID:            1,
		Name:                 i18n.New(nameAr, nameEn),
		Latitude:             lat,
		Longitude:            lon,
		CoverageRadiusMeters: govRadiusMeters,
		IsActive:             true,
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.CreateGovernorate(ctx, gov); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/cities", "success", i18n.T(lang, "admin.geo.gov_created_success"))
}

// AdminGovernorateEditSubmit updates an existing governorate.
func (h *UIHandler) AdminGovernorateEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.gov_invalid_id"))
		return
	}

	_ = r.ParseForm()
	nameAr := strings.TrimSpace(r.PostFormValue("gov_name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("gov_name_en"))
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.gov_name_ar_required"))
		return
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("gov_lat"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("gov_lon"), 64)
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == "on"

	govRadiusMeters, _ := strconv.Atoi(r.PostFormValue("gov_coverage_radius_meters"))
	if govRadiusMeters <= 0 {
		govRadiusMeters, _ = strconv.Atoi(r.PostFormValue("coverage_radius_meters"))
	}
	if govRadiusMeters <= 0 {
		govRadiusMeters, _ = strconv.Atoi(r.PostFormValue("radius"))
	}
	if govRadiusMeters <= 0 {
		govRadiusMeters = 25000
	}

	gov := &platformadmin.Governorate{
		ID:                   id,
		CountryID:            1,
		Name:                 i18n.New(nameAr, nameEn),
		Latitude:             lat,
		Longitude:            lon,
		CoverageRadiusMeters: govRadiusMeters,
		IsActive:             isActive,
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.UpdateGovernorate(ctx, gov); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/admin/cities"
	}
	h.redirectWithNotice(w, r, referer, "success", i18n.T(lang, "admin.geo.gov_updated_success"))
}

// AdminGovernorateToggleSubmit toggles the active status of a governorate.
func (h *UIHandler) AdminGovernorateToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.gov_invalid_id"))
		return
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.ToggleGovernorateStatus(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/cities", "success", i18n.T(lang, "admin.geo.gov_status_updated_success"))
}
