package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminCitiesPage renders the Egyptian cities and spatial coordinates management screen.
func (h *UIHandler) AdminCitiesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var governorates []*platformadmin.Governorate
	var allCities []*platformadmin.City
	if h.adminSvc != nil {
		governorates, _ = h.adminSvc.ListAllGovernorates(ctx, 1)
		allCities, _ = h.adminSvc.ListAllCities(database.AsSystem(ctx), 1)
	}

	selectedGovID, _ := strconv.ParseInt(r.URL.Query().Get("gov_id"), 10, 64)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	qLower := strings.ToLower(query)
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))

	var filteredCities []*platformadmin.City
	for _, c := range allCities {
		if c == nil {
			continue
		}
		if selectedGovID > 0 && (c.GovernorateID == nil || *c.GovernorateID != selectedGovID) {
			continue
		}
		if statusFilter == "active" && !c.IsActive {
			continue
		}
		if statusFilter == "inactive" && c.IsActive {
			continue
		}
		if query != "" {
			var nameAr, nameEn string
			nameAr = c.Name["ar"]
			nameEn = strings.ToLower(c.Name["en"])

			var govAr, govEn string
			if c.GovernorateName != nil {
				govAr = (*c.GovernorateName)["ar"]
				govEn = strings.ToLower((*c.GovernorateName)["en"])
			}
			if !strings.Contains(nameAr, query) && !strings.Contains(nameEn, qLower) &&
				!strings.Contains(govAr, query) && !strings.Contains(govEn, qLower) {
				continue
			}
		}
		filteredCities = append(filteredCities, c)
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)

	totalFiltered := len(filteredCities)
	var paginatedCities []*platformadmin.City
	var totalPages int = 1

	if limit >= 1000 {
		paginatedCities = filteredCities
		totalPages = 1
	} else {
		totalPages = (totalFiltered + limit - 1) / limit
		if totalPages < 1 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}
		start := (page - 1) * limit
		if start < 0 {
			start = 0
		}
		end := start + limit
		if end > totalFiltered {
			end = totalFiltered
		}
		if start < totalFiltered {
			paginatedCities = filteredCities[start:end]
		}
	}

	data := pages.AdminCitiesData{
		Governorates:          governorates,
		Cities:                paginatedCities,
		SelectedGovernorateID: selectedGovID,
		TotalCities:           len(allCities),
		TotalGovernorates:     len(governorates),
		TotalFiltered:         totalFiltered,
		Page:                  page,
		Limit:                 limit,
		TotalPages:            totalPages,
		Query:                 query,
		Status:                statusFilter,
		QueryValues:           r.URL.Query(),
	}

	h.renderPage(ctx, w, "render admin cities page", pages.AdminCities(data, lang, dir, h.isHTMX(r)))
}

// AdminCityCreateSubmit adds a new city / district with coordinates under a governorate.
func (h *UIHandler) AdminCityCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	_ = r.ParseForm()
	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.city_name_ar_required"))
		return
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	var govIDPtr *int64
	govID, _ := strconv.ParseInt(r.PostFormValue("governorate_id"), 10, 64)
	if govID > 0 {
		govIDPtr = &govID
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("city_lat"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("city_lon"), 64)

	radiusMeters, _ := strconv.Atoi(r.PostFormValue("coverage_radius_meters"))
	if radiusMeters <= 0 {
		radiusMeters, _ = strconv.Atoi(r.PostFormValue("radius"))
	}
	if radiusMeters <= 0 {
		radiusMeters = platformadmin.DefaultCoverageRadiusMeters
	}

	city := &platformadmin.City{
		CountryID:            1,
		GovernorateID:        govIDPtr,
		Name:                 i18n.New(nameAr, nameEn),
		Latitude:             lat,
		Longitude:            lon,
		CoverageRadiusMeters: radiusMeters,
		IsActive:             true,
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.CreateCity(ctx, city); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	redirectURL := "/admin/cities"
	if govID > 0 {
		redirectURL = fmt.Sprintf("/admin/cities?gov_id=%d", govID)
	}
	h.redirectWithNotice(w, r, redirectURL, "success", i18n.T(lang, "admin.geo.city_created_success"))
}

// AdminCityToggleSubmit toggles the active status of a city.
func (h *UIHandler) AdminCityToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.city_invalid_id"))
		return
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.ToggleCityStatus(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/admin/cities"
	}
	h.redirectWithNotice(w, r, referer, "success", i18n.T(lang, "admin.geo.city_status_updated_success"))
}

// AdminCityEditSubmit updates an existing city / district.
func (h *UIHandler) AdminCityEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.city_invalid_id"))
		return
	}

	_ = r.ParseForm()
	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.city_name_ar_required"))
		return
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	var govIDPtr *int64
	govID, _ := strconv.ParseInt(r.PostFormValue("governorate_id"), 10, 64)
	if govID > 0 {
		govIDPtr = &govID
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("city_lat"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("city_lon"), 64)
	isCapital := r.PostFormValue("is_capital") == "true" || r.PostFormValue("is_capital") == "1" || r.PostFormValue("is_capital") == "on"
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == "on"

	radiusMeters, _ := strconv.Atoi(r.PostFormValue("coverage_radius_meters"))
	if radiusMeters <= 0 {
		radiusMeters, _ = strconv.Atoi(r.PostFormValue("radius"))
	}
	if radiusMeters <= 0 {
		radiusMeters = platformadmin.DefaultCoverageRadiusMeters
	}

	city := &platformadmin.City{
		ID:                   id,
		CountryID:            1,
		GovernorateID:        govIDPtr,
		Name:                 i18n.New(nameAr, nameEn),
		Latitude:             lat,
		Longitude:            lon,
		CoverageRadiusMeters: radiusMeters,
		IsActive:             isActive,
		IsCapital:            isCapital,
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.UpdateCity(ctx, city); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/admin/cities"
	}
	h.redirectWithNotice(w, r, referer, "success", i18n.T(lang, "admin.geo.city_updated_success"))
}
