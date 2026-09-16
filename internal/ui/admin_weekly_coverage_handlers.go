package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminWeeklyCoveragesPage renders weekly delivery coverages filtered by organization dropdown.
func (h *UIHandler) AdminWeeklyCoveragesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var orgs []*org.Organization
	if h.orgSvc != nil {
		orgs, _ = h.orgSvc.ListOrganizations(database.AsSystem(ctx), nil, nil, 200, 0)
	}

	var selectedOrgID int64
	if orgIDStr := r.URL.Query().Get("org_id"); orgIDStr != "" {
		selectedOrgID, _ = strconv.ParseInt(orgIDStr, 10, 64)
	}

	var coverages []*workflow.CoverageView
	var totalCount int
	if h.wfSvc != nil {
		// AsSystem justified: platform admin viewing weekly coverages across tenants
		cList, total, err := h.wfSvc.ListCoverageForOrganizationWithTotal(database.AsSystem(ctx), selectedOrgID, limit, offset)
		if err == nil {
			coverages = cList
			totalCount = total
		}
	}

	var branches []*org.Branch
	if h.orgSvc != nil {
		if selectedOrgID > 0 {
			branches, _ = h.orgSvc.ListBranches(database.AsSystem(ctx), selectedOrgID)
		} else {
			branches, _ = h.orgSvc.ListBranches(database.AsSystem(ctx), 0)
		}
	}

	var cities []*platformadmin.City
	if h.adminSvc != nil {
		cities, _ = h.adminSvc.ListCities(database.AsSystem(ctx), 1)
	}

	data := pages.AdminWeeklyCoveragesData{
		Coverages:     coverages,
		Organizations: orgs,
		Branches:      branches,
		Cities:        cities,
		SelectedOrgID: selectedOrgID,
		Page:          page,
		PerPage:       limit,
		TotalCount:    totalCount,
	}

	h.renderPage(ctx, w, "render admin weekly coverages", pages.AdminWeeklyCoveragesPage(data, lang, dir))
}

// AdminWeeklyCoverageCreateSubmit creates a new weekly coverage schedule.
func (h *UIHandler) AdminWeeklyCoverageCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = r.ParseForm()

	orgID, _ := strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	branchID, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}
	dayOfWeek, _ := strconv.Atoi(r.PostFormValue("day_of_week"))
	distKm, _ := strconv.Atoi(r.PostFormValue("distance_km"))
	if distKm <= 0 {
		distKm = 50
	}
	address := r.PostFormValue("address")

	var covFrom, covTo *string
	if f := r.PostFormValue("coverage_from"); f != "" {
		covFrom = &f
	}
	if t := r.PostFormValue("coverage_to"); t != "" {
		covTo = &t
	}

	cov := workflow.WeeklyCoverage{
		OrganizationID: orgID,
		BranchID:       branchID,
		CityID:         cityID,
		DayOfWeek:      dayOfWeek,
		CoverageFrom:   covFrom,
		CoverageTo:     covTo,
		DistanceMeters: distKm * 1000,
		Address:        address,
		IsActive:       true,
	}

	redirectOrgID := r.PostFormValue("redirect_org_id")

	if h.wfSvc != nil {
		if err := h.wfSvc.CreateWeeklyCoverage(database.AsSystem(ctx), &cov); err != nil {
			h.log.ErrorContext(ctx, "create weekly coverage", "error", err)
			h.redirectWithNotice(w, r, "/admin/weekly-coverages?org_id="+redirectOrgID, "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/weekly-coverages?org_id="+redirectOrgID, "success", i18n.T(langOf(r), "admin.org.coverage_created_success"))
}

// AdminWeeklyCoverageToggleSubmit toggles the active status of a coverage schedule.
func (h *UIHandler) AdminWeeklyCoverageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	isActive := r.PostFormValue("is_active") == "true"
	orgID := r.PostFormValue("org_id")

	if h.wfSvc != nil && id > 0 {
		_ = h.wfSvc.ToggleWeeklyCoverage(database.AsSystem(ctx), id, isActive)
	}

	h.redirectWithNotice(w, r, "/admin/weekly-coverages?org_id="+orgID, "success", i18n.T(langOf(r), "admin.org.coverage_status_updated_success"))
}

// AdminWeeklyCoverageDeleteSubmit deletes a coverage schedule.
func (h *UIHandler) AdminWeeklyCoverageDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	orgID := r.PostFormValue("org_id")

	if h.wfSvc != nil && id > 0 {
		_ = h.wfSvc.DeleteWeeklyCoverage(database.AsSystem(ctx), id)
	}

	h.redirectWithNotice(w, r, "/admin/weekly-coverages?org_id="+orgID, "success", i18n.T(langOf(r), "admin.org.coverage_deleted_success"))
}
