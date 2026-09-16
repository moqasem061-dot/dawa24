package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) getBranchWithWorks(ctx context.Context, branchID int64) (*org.Branch, []*org.InstitutionalWork, []*org.InstitutionalWork) {
	if h.orgSvc == nil {
		return nil, nil, nil
	}
	sysCtx := database.AsSystem(ctx)
	b, err := h.orgSvc.GetBranch(sysCtx, branchID)
	if err != nil || b == nil {
		return nil, nil, nil
	}
	assigned, _ := h.orgSvc.GetBranchInstitutionalWorks(sysCtx, branchID)
	reachable, _ := h.orgSvc.GetReachableBuyerWorksForBranch(sysCtx, branchID)
	return b, assigned, reachable
}

// AdminBranchDetailPage renders detail for a specific branch.
func (h *UIHandler) AdminBranchDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	branchID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	branch, assigned, reachable := h.getBranchWithWorks(ctx, branchID)
	if branch == nil {
		http.Redirect(w, r, "/admin/branches", http.StatusSeeOther)
		return
	}
	h.renderPage(ctx, w, "render admin branch detail", pages.AdminBranchDetailPage(branch, assigned, reachable, lang, dir))
}

// AdminBranchProductsPage renders catalog assigned to a branch.
func (h *UIHandler) AdminBranchProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	branchID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	branch, assigned, reachable := h.getBranchWithWorks(ctx, branchID)
	if branch == nil {
		http.Redirect(w, r, "/admin/branches", http.StatusSeeOther)
		return
	}
	h.renderPage(ctx, w, "render admin branch products", pages.AdminBranchDetailPage(branch, assigned, reachable, lang, dir))
}

// AdminBranchUsersPage renders staff assigned to a branch.
func (h *UIHandler) AdminBranchUsersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	branchID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	branch, assigned, reachable := h.getBranchWithWorks(ctx, branchID)
	if branch == nil {
		http.Redirect(w, r, "/admin/branches", http.StatusSeeOther)
		return
	}
	h.renderPage(ctx, w, "render admin branch users", pages.AdminBranchDetailPage(branch, assigned, reachable, lang, dir))
}

// AdminBranchNewSubmit creates a new branch or warehouse for any organization.
func (h *UIHandler) AdminBranchNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/branches", "error", i18n.T(lang, "customer.branch.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	orgID, err := strconv.ParseInt(r.PostFormValue("org_id"), 10, 64)
	if err != nil || orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "يرجى تحديد المنشأة التابع لها الفرع")
		return
	}

	var orgType string
	if targetOrg, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && targetOrg != nil {
		orgType = string(targetOrg.Type)
	}

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		if orgType == string(org.TypeCustomer) {
			nameAr = "فرع صيدلية جديد"
		} else {
			nameAr = "مستودع جديد"
		}
	}
	if nameEn == "" {
		nameEn = nameAr
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	if code == "" {
		code = fmt.Sprintf("BR-%d", time.Now().Unix())
	}
	warehouseType := strings.TrimSpace(r.PostFormValue("warehouse_type"))
	if orgType == string(org.TypeCustomer) {
		warehouseType = "pharmacy"
	} else {
		if warehouseType == "" || warehouseType == "branch" || warehouseType == "pharmacy" {
			warehouseType = "warehouse"
		}
	}
	capacitySQM, _ := strconv.ParseFloat(r.PostFormValue("capacity_sqm"), 64)
	address := strings.TrimSpace(r.PostFormValue("address"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	operatingHours := strings.TrimSpace(r.PostFormValue("operating_hours"))
	gmaps := strings.TrimSpace(r.PostFormValue("google_maps_url"))
	isMain := r.PostFormValue("is_main") == "true" || r.PostFormValue("is_main") == "on" || r.PostFormValue("is_main") == "1"
	hasColdStorage := r.PostFormValue("has_cold_storage") == "true" || r.PostFormValue("has_cold_storage") == "on" || r.PostFormValue("has_cold_storage") == "1"

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	var latPtr, lngPtr *float64
	if latStr := r.PostFormValue("latitude"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			latPtr = &lat
		}
	}
	if lngStr := r.PostFormValue("longitude"); lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lngPtr = &lng
		}
	}

	b := &org.Branch{
		OrganizationID:     orgID,
		Name:               i18n.New(nameAr, nameEn),
		Code:               code,
		WarehouseType:      warehouseType,
		CapacitySQM:        capacitySQM,
		Address:            address,
		Phone:              phone,
		OperatingHours:     operatingHours,
		HasColdStorage:     hasColdStorage,
		GoogleMapsURL:      gmaps,
		CityID:             cityID,
		Latitude:           latPtr,
		Longitude:          lngPtr,
		IsMain:             isMain,
		Status:             "active",
		InstitutionalWorks: r.Form["institutional_works"],
	}

	if err := h.orgSvc.CreateBranch(database.AsSystem(ctx), b); err != nil {
		h.log.ErrorContext(ctx, "admin create branch error", "error", err)
		h.redirectWithNotice(w, r, "/admin/branches", "error", h.safeMessage(err, lang))
		return
	}
	InvalidateBranchOptionsCache(orgID)

	h.redirectWithNotice(w, r, "/admin/branches", "success", "تمت إضافة الفرع بنجاح.")
}

// AdminBranchEditSubmit updates an existing branch.
func (h *UIHandler) AdminBranchEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/branches", "error", i18n.T(lang, "customer.branch.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "معرف الفرع غير صالح")
		return
	}

	existing, err := h.orgSvc.GetBranch(database.AsSystem(ctx), id)
	if err != nil || existing == nil {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "لم يتم العثور على الفرع المطلوب")
		return
	}

	_ = r.ParseForm()

	orgID, _ := strconv.ParseInt(r.PostFormValue("org_id"), 10, 64)
	if orgID <= 0 {
		orgID = existing.OrganizationID
	}

	var orgType string
	if targetOrg, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && targetOrg != nil {
		orgType = string(targetOrg.Type)
	}

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		nameAr = existing.Name.Get(i18n.AR)
	}
	if nameEn == "" {
		nameEn = nameAr
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	if code == "" {
		code = existing.Code
	}
	warehouseType := strings.TrimSpace(r.PostFormValue("warehouse_type"))
	if orgType == string(org.TypeCustomer) {
		warehouseType = "pharmacy"
	} else {
		if warehouseType == "" {
			warehouseType = existing.WarehouseType
		}
		if warehouseType == "" || warehouseType == "branch" || warehouseType == "pharmacy" {
			warehouseType = "warehouse"
		}
	}
	capSQMVal := strings.TrimSpace(r.PostFormValue("capacity_sqm"))
	var capacitySQM float64
	if capSQMVal != "" {
		capacitySQM, _ = strconv.ParseFloat(capSQMVal, 64)
	} else if orgType != string(org.TypeCustomer) {
		capacitySQM = existing.CapacitySQM
	}
	address := strings.TrimSpace(r.PostFormValue("address"))
	if address == "" {
		address = existing.Address
	}
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	if phone == "" {
		phone = existing.Phone
	}
	operatingHours := strings.TrimSpace(r.PostFormValue("operating_hours"))
	gmaps := strings.TrimSpace(r.PostFormValue("google_maps_url"))
	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = existing.Status
	}
	isMain := r.PostFormValue("is_main") == "true" || r.PostFormValue("is_main") == "on" || r.PostFormValue("is_main") == "1"
	hasColdStorage := r.PostFormValue("has_cold_storage") == "true" || r.PostFormValue("has_cold_storage") == "on" || r.PostFormValue("has_cold_storage") == "1"

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	} else {
		cityID = existing.CityID
	}

	var latPtr, lngPtr *float64
	if latStr := r.PostFormValue("latitude"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			latPtr = &lat
		}
	}
	if lngStr := r.PostFormValue("longitude"); lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lngPtr = &lng
		}
	}
	if latPtr == nil {
		latPtr = existing.Latitude
	}
	if lngPtr == nil {
		lngPtr = existing.Longitude
	}

	b := &org.Branch{
		ID:                 id,
		OrganizationID:     orgID,
		Name:               i18n.New(nameAr, nameEn),
		Code:               code,
		WarehouseType:      warehouseType,
		CapacitySQM:        capacitySQM,
		Address:            address,
		Phone:              phone,
		OperatingHours:     operatingHours,
		HasColdStorage:     hasColdStorage,
		GoogleMapsURL:      gmaps,
		CityID:             cityID,
		Latitude:           latPtr,
		Longitude:          lngPtr,
		IsMain:             isMain,
		Status:             status,
		InstitutionalWorks: r.Form["institutional_works"],
	}

	if err := h.orgSvc.UpdateBranch(database.AsSystem(ctx), b); err != nil {
		h.log.ErrorContext(ctx, "admin update branch error", "error", err, "branch_id", id)
		h.redirectWithNotice(w, r, "/admin/branches", "error", h.safeMessage(err, lang))
		return
	}
	InvalidateBranchOptionsCache(orgID)

	h.redirectWithNotice(w, r, "/admin/branches", "success", "تم تحديث بيانات الفرع بنجاح.")
}

// AdminBranchDeleteSubmit removes a branch.
func (h *UIHandler) AdminBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/branches", "error", i18n.T(lang, "customer.branch.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "معرف الفرع غير صالح")
		return
	}

	existing, err := h.orgSvc.GetBranch(database.AsSystem(ctx), id)
	if err != nil || existing == nil {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "لم يتم العثور على الفرع المطلوب")
		return
	}
	if existing.IsMain {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "لا يمكن حذف الفرع الرئيسي للمنشأة")
		return
	}

	if err := h.orgSvc.DeleteBranch(database.AsSystem(ctx), id, existing.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "admin delete branch error", "error", err, "branch_id", id)
		h.redirectWithNotice(w, r, "/admin/branches", "error", h.safeMessage(err, lang))
		return
	}
	InvalidateBranchOptionsCache(existing.OrganizationID)

	h.redirectWithNotice(w, r, "/admin/branches", "success", "تم حذف الفرع بنجاح.")
}
