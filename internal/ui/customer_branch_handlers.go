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
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerBranchCreatePage redirects to the unified branches page in add mode.
func (h *UIHandler) CustomerBranchCreatePage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/customer/branches", http.StatusSeeOther)
}

// CustomerBranchEditPage redirects to the unified branches page with edit mode preloaded.
func (h *UIHandler) CustomerBranchEditPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id != "" {
		http.Redirect(w, r, "/customer/branches?edit="+id, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/customer/branches", http.StatusSeeOther)
}

// CustomerBranchesPage renders the pharmacy's branch network.
//
// It used to carry the employees list as a second tab, load every member of the
// company to print a head count beside each branch card, and redirect its own
// write actions back to ?tab=employees. The tab moved to /customer/team; the
// head count is now one GROUP BY.
func (h *UIHandler) CustomerBranchesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	// Bookmarks and every pre-existing redirect pointed at ?tab=employees.
	if r.URL.Query().Get("tab") == "employees" {
		http.Redirect(w, r, "/customer/team", http.StatusMovedPermanently)
		return
	}

	var branches []*org.Branch
	staff := map[int64]int{}
	total := 0
	var instWorks []*org.InstitutionalWork
	if h.orgSvc != nil {
		var err error
		if branches, err = h.orgSvc.ListBranches(ctx, orgID); err != nil {
			h.log.ErrorContext(ctx, "list pharmacy branches", "error", err, "organization_id", orgID)
		}
		if staff, err = h.orgSvc.CountMembersByBranch(ctx, orgID); err != nil {
			h.log.ErrorContext(ctx, "count members by branch", "error", err, "organization_id", orgID)
			staff = map[int64]int{}
		}
		for _, n := range staff {
			total += n
		}
		if instWorks, err = h.orgSvc.ListAllFlatInstitutionalWorks(ctx, true); err != nil {
			h.log.ErrorContext(ctx, "list pharmacy institutional works", "error", err)
		}
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("notice_msg")
	}

	data := pages.CustomerBranchesData{
		Branches:           branches,
		StaffPerBranch:     staff,
		TotalStaff:         total,
		Governorates:       h.listGovernorates(ctx),
		Cities:             h.listCities(ctx),
		InstitutionalWorks: instWorks,
		NoticeType:         noticeType,
		NoticeMsg:          noticeMsg,
	}

	h.renderPage(ctx, w, "render customer branches page", pages.CustomerBranches(data, lang, dir, actor.Permissions))
}

// CustomerBranchNewSubmit creates a new pharmacy branch for the customer organization.
func (h *UIHandler) CustomerBranchNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		nameAr = i18n.T(langOf(r), "customer.branch.default_name")
	}
	if nameEn == "" {
		nameEn = nameAr
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	if code == "" {
		code = fmt.Sprintf("BR-%d", time.Now().Unix())
	}
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
		OrganizationID:     actor.OrganizationID,
		Name:               i18n.New(nameAr, nameEn),
		Code:               code,
		WarehouseType:      "pharmacy",
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

	if h.orgSvc != nil {
		if err := h.orgSvc.CreateBranch(ctx, b); err != nil {
			h.log.ErrorContext(ctx, "customer create branch error", "error", err)
			h.redirectWithNotice(w, r, "/customer/branches", "error", h.safeMessage(err, langOf(r)))
			return
		}
		// The selector's option list is cached per organisation for a few seconds
		// (see branchCacheTTL); drop it so the person who just made this change
		// sees it on the next page rather than up to thirty seconds later.
		InvalidateBranchOptionsCache(actor.OrganizationID)
	}

	h.notifyBranchCreated(ctx, actor.OrganizationID, nameAr, code)
	h.redirectWithNotice(w, r, "/customer/branches", "success", i18n.T(langOf(r), "customer.branch.create_success"))
}

// CustomerBranchEditSubmit updates an existing pharmacy branch.
func (h *UIHandler) CustomerBranchEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches", "error", i18n.T(langOf(r), "customer.branch.invalid_id"))
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/customer/branches", "error", i18n.T(langOf(r), "customer.branch.service_unavailable"))
		return
	}

	existing, err := h.orgSvc.GetBranch(ctx, id)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/customer/branches", "error", i18n.T(langOf(r), "customer.branch.not_found_edit"))
		return
	}

	_ = r.ParseForm()

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
	if code == "" {
		code = fmt.Sprintf("BR-%d", id)
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
		status = "active"
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
		OrganizationID:     actor.OrganizationID,
		Name:               i18n.New(nameAr, nameEn),
		Code:               code,
		WarehouseType:      "pharmacy",
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

	if err := h.orgSvc.UpdateBranch(ctx, b); err != nil {
		h.log.ErrorContext(ctx, "customer edit branch error", "error", err, "branch_id", id)
		h.redirectWithNotice(w, r, "/customer/branches", "error", h.safeMessage(err, langOf(r)))
		return
	}
	// The selector's option list is cached per organisation for a few seconds
	// (see branchCacheTTL); drop it so the person who just made this change
	// sees it on the next page rather than up to thirty seconds later.
	InvalidateBranchOptionsCache(actor.OrganizationID)

	if status == "inactive" || status == "disabled" {
		h.notifyBranchDisabled(ctx, actor.OrganizationID, nameAr)
	}
	if len(b.InstitutionalWorks) > 0 {
		h.notifyBranchInstitutionalWorksChanged(ctx, actor.OrganizationID, nameAr)
	}

	h.redirectWithNotice(w, r, "/customer/branches", "success", i18n.T(langOf(r), "customer.branch.update_success"))
}

// CustomerBranchDeleteSubmit deletes a branch owned by the customer organization.
func (h *UIHandler) CustomerBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches", "error", i18n.T(langOf(r), "customer.branch.invalid_id"))
		return
	}

	if h.orgSvc != nil {
		existing, err := h.orgSvc.GetBranch(ctx, id)
		if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
			h.redirectWithNotice(w, r, "/customer/branches", "error", i18n.T(langOf(r), "customer.branch.not_found_delete"))
			return
		}
		if existing.IsMain {
			h.redirectWithNotice(w, r, "/customer/branches", "error", i18n.T(langOf(r), "customer.branch.cannot_delete_main"))
			return
		}
		if err := h.orgSvc.DeleteBranch(ctx, id, actor.OrganizationID); err != nil {
			h.redirectWithNotice(w, r, "/customer/branches", "error", h.safeMessage(err, langOf(r)))
			return
		}
		// The selector's option list is cached per organisation for a few seconds
		// (see branchCacheTTL); drop it so the person who just made this change
		// sees it on the next page rather than up to thirty seconds later.
		InvalidateBranchOptionsCache(actor.OrganizationID)
	}

	h.redirectWithNotice(w, r, "/customer/branches", "success", i18n.T(langOf(r), "customer.branch.delete_success"))
}

// CustomerSwitchActiveBranchSubmit handles switching the active branch for customer context.
func (h *UIHandler) CustomerSwitchActiveBranchSubmit(w http.ResponseWriter, r *http.Request) {
	h.SetBuyingBranchSubmit(w, r)
}
