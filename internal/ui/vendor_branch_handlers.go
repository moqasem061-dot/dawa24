package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorBranchesPage renders the detailed branch management view.
func (h *UIHandler) VendorBranchesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/branches", http.StatusSeeOther)
		return
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var branches []*org.Branch
	var employees []*org.EmployeeView
	var instWorks []*org.InstitutionalWork
	var total int
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		branches, total, _ = h.orgSvc.ListBranchesWithTotal(ctx, org.BranchFilter{OrganizationID: actor.OrganizationID}, limit, offset)
		employees, _ = h.orgSvc.ListEmployees(ctx, actor.OrganizationID)
		instWorks, _ = h.orgSvc.ListAllFlatInstitutionalWorks(ctx, true)
	}

	data := pages.VendorBranchesData{
		Branches:           branches,
		Governorates:       h.listGovernorates(ctx),
		Cities:             h.listCities(ctx),
		Employees:          employees,
		InstitutionalWorks: instWorks,
		Page:               page,
		PerPage:            limit,
		TotalCount:         total,
	}

	h.renderPage(ctx, w, "render vendor branches page", pages.VendorBranchesPage(data, lang, dir))
}

// VendorBranchNewSubmit creates a new physical branch or warehouse.
func (h *UIHandler) VendorBranchNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/branches", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.log.WarnContext(ctx, "parse form error", "error", err)
	}

	nameAr := r.PostFormValue("name_ar")
	nameEn := r.PostFormValue("name_en")
	if nameEn == "" {
		nameEn = nameAr
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	if code == "" {
		code = fmt.Sprintf("BR-%d", time.Now().Unix())
	}
	warehouseType := r.PostFormValue("warehouse_type")
	address := r.PostFormValue("address")
	phone := r.PostFormValue("phone")
	gmaps := r.PostFormValue("google_maps_url")
	hours := r.PostFormValue("operating_hours")
	hasCold := r.PostFormValue("has_cold_storage") == "true"
	isMain := r.PostFormValue("is_main") == "true"

	managerIDVal, _ := strconv.ParseInt(r.PostFormValue("manager_id"), 10, 64)
	var managerID *int64
	if managerIDVal > 0 {
		managerID = &managerIDVal
	}

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	capSQM, _ := strconv.ParseFloat(r.PostFormValue("capacity_sqm"), 64)

	var latPtr, lngPtr *float64
	latStr := r.PostFormValue("latitude")
	if latStr == "" {
		latStr = r.PostFormValue("branch_lat")
	}
	if latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			latPtr = &lat
		}
	}

	lngStr := r.PostFormValue("longitude")
	if lngStr == "" {
		lngStr = r.PostFormValue("branch_lon")
	}
	if lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lngPtr = &lng
		}
	}

	if gmaps == "" {
		gmaps = r.PostFormValue("branch_google_maps_url")
	}

	instWorks := r.Form["institutional_works"]

	var managerName string
	if managerID != nil && h.idSvc != nil {
		if u, err := h.idSvc.GetUserByID(ctx, *managerID); err == nil && u != nil {
			managerName = u.Name.Get("ar")
			if managerName == "" {
				managerName = u.Name.Get("en")
			}
			if managerName == "" {
				managerName = u.Email
			}
		}
	}

	b := &org.Branch{
		OrganizationID:     actor.OrganizationID,
		Name:               i18n.New(nameAr, nameEn),
		Code:               code,
		WarehouseType:      warehouseType,
		Address:            address,
		Phone:              phone,
		ManagerID:          managerID,
		ManagerName:        managerName,
		GoogleMapsURL:      gmaps,
		OperatingHours:     hours,
		HasColdStorage:     hasCold,
		CapacitySQM:        capSQM,
		CityID:             cityID,
		Latitude:           latPtr,
		Longitude:          lngPtr,
		IsMain:             isMain,
		Status:             "active",
		InstitutionalWorks: instWorks,
	}

	if h.orgSvc != nil {
		if err := h.orgSvc.CreateBranch(ctx, b); err != nil {
			h.log.ErrorContext(ctx, "create branch error", "error", err)
			h.redirectWithNotice(w, r, "/vendor/branches", "error", h.safeMessage(err, langOf(r)))
			return
		}
		// The selector's option list is cached per organisation for a few seconds
		// (see branchCacheTTL); drop it so the person who just made this change
		// sees it on the next page rather than up to thirty seconds later.
		InvalidateBranchOptionsCache(actor.OrganizationID)
	}

	h.notifyBranchCreated(ctx, actor.OrganizationID, nameAr, code)
	h.redirectWithNotice(w, r, "/vendor/branches", "success", i18n.T(langOf(r), "vendor.branch.create_success"))
}

// VendorBranchEditPage redirects to the unified vendor branches page with edit mode preloaded.
func (h *UIHandler) VendorBranchEditPage(w http.ResponseWriter, r *http.Request) {
	branchID := chi.URLParam(r, "id")
	if branchID != "" {
		http.Redirect(w, r, "/vendor/branches?edit="+branchID, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/vendor/branches", http.StatusSeeOther)
}

// VendorBranchEditSubmit saves updates to an existing branch.
func (h *UIHandler) VendorBranchEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/branches", http.StatusSeeOther)
		return
	}

	branchID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || branchID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", i18n.T(langOf(r), "vendor.branch.invalid_id"))
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", i18n.T(langOf(r), "vendor.branch.org_service_unavailable"))
		return
	}

	existing, err := h.orgSvc.GetBranch(ctx, branchID)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", i18n.T(langOf(r), "vendor.branch.not_found_edit"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.log.WarnContext(ctx, "parse form error", "error", err)
	}

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/branches/%d/edit", branchID), "error", i18n.T(langOf(r), "vendor.branch.name_ar_required"))
		return
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	code := strings.TrimSpace(r.PostFormValue("code"))
	if code == "" {
		code = existing.Code
	}
	if code == "" {
		code = fmt.Sprintf("BR-%d", branchID)
	}
	warehouseType := strings.TrimSpace(r.PostFormValue("warehouse_type"))
	if warehouseType == "" {
		warehouseType = existing.WarehouseType
	}
	if warehouseType == "" {
		warehouseType = "warehouse"
	}
	address := strings.TrimSpace(r.PostFormValue("address"))
	if address == "" {
		address = existing.Address
	}
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	if phone == "" {
		phone = existing.Phone
	}
	gmaps := strings.TrimSpace(r.PostFormValue("google_maps_url"))
	hours := strings.TrimSpace(r.PostFormValue("operating_hours"))
	hasCold := r.PostFormValue("has_cold_storage") == "true"
	isMain := r.PostFormValue("is_main") == "true"
	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = "active"
	}

	managerIDVal, _ := strconv.ParseInt(r.PostFormValue("manager_id"), 10, 64)
	var managerID *int64
	if managerIDVal > 0 {
		managerID = &managerIDVal
	}

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	capSQM, _ := strconv.ParseFloat(r.PostFormValue("capacity_sqm"), 64)

	var latPtr, lngPtr *float64
	latStr := strings.TrimSpace(r.PostFormValue("latitude"))
	if latStr == "" {
		latStr = strings.TrimSpace(r.PostFormValue("branch_lat"))
	}
	if latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			latPtr = &lat
		}
	}

	lngStr := strings.TrimSpace(r.PostFormValue("longitude"))
	if lngStr == "" {
		lngStr = strings.TrimSpace(r.PostFormValue("branch_lon"))
	}
	if lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lngPtr = &lng
		}
	}

	if gmaps == "" {
		gmaps = strings.TrimSpace(r.PostFormValue("branch_google_maps_url"))
	}

	instWorks := r.Form["institutional_works"]

	var managerName string
	if managerID != nil && h.idSvc != nil {
		if u, err := h.idSvc.GetUserByID(ctx, *managerID); err == nil && u != nil {
			managerName = u.Name.Get("ar")
			if managerName == "" {
				managerName = u.Name.Get("en")
			}
			if managerName == "" {
				managerName = u.Email
			}
		}
	}

	existing.Name = i18n.New(nameAr, nameEn)
	existing.Code = code
	existing.WarehouseType = warehouseType
	existing.Address = address
	existing.Phone = phone
	existing.ManagerID = managerID
	existing.ManagerName = managerName
	existing.GoogleMapsURL = gmaps
	existing.OperatingHours = hours
	existing.HasColdStorage = hasCold
	existing.CapacitySQM = capSQM
	existing.CityID = cityID
	existing.Latitude = latPtr
	existing.Longitude = lngPtr
	existing.IsMain = isMain
	existing.Status = status
	existing.InstitutionalWorks = instWorks

	if err := h.orgSvc.UpdateBranch(ctx, existing); err != nil {
		h.log.ErrorContext(ctx, "update branch error", "error", err)
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/branches/%d/edit", branchID), "error", h.safeMessage(err, langOf(r)))
		return
	}
	// The selector's option list is cached per organisation for a few seconds
	// (see branchCacheTTL); drop it so the person who just made this change
	// sees it on the next page rather than up to thirty seconds later.
	InvalidateBranchOptionsCache(actor.OrganizationID)

	if status == "inactive" || status == "disabled" {
		h.notifyBranchDisabled(ctx, actor.OrganizationID, nameAr)
	}
	if len(instWorks) > 0 {
		h.notifyBranchInstitutionalWorksChanged(ctx, actor.OrganizationID, nameAr)
	}

	h.redirectWithNotice(w, r, "/vendor/branches", "success", i18n.T(langOf(r), "vendor.branch.update_success"))
}

// VendorBranchDeleteSubmit deletes a branch scoped to the vendor organization.
func (h *UIHandler) VendorBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", i18n.T(langOf(r), "vendor.branch.invalid_id_alt"))
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", i18n.T(langOf(r), "vendor.branch.orgs_service_unavailable"))
		return
	}
	existing, err := h.orgSvc.GetBranch(ctx, id)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", i18n.T(langOf(r), "vendor.branch.not_found_edit"))
		return
	}
	if existing.IsMain {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", i18n.T(langOf(r), "customer.branch.cannot_delete_main"))
		return
	}
	if err := h.orgSvc.DeleteBranch(ctx, id, actor.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "delete branch", "error", err, "branch", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/branches", "error", h.safeMessage(err, langOf(r)))
		return
	}
	// The selector's option list is cached per organisation for a few seconds
	// (see branchCacheTTL); drop it so the person who just made this change
	// sees it on the next page rather than up to thirty seconds later.
	InvalidateBranchOptionsCache(actor.OrganizationID)
	h.redirectWithNotice(w, r, "/vendor/branches", "success", i18n.T(langOf(r), "customer.branch.delete_success"))
}
