package ui

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// VendorCoverageUpdateSubmit processes updates to a single weekly coverage record.
func (h *UIHandler) VendorCoverageUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	}
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("coverage_id"), 10, 64)
	}
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.invalid_id"))
		return
	}

	existingCov, err := h.wfSvc.GetWeeklyCoverage(ctx, id)
	if err != nil || existingCov == nil || existingCov.OrganizationID != actor.OrganizationID {
		h.log.WarnContext(ctx, "cross-tenant coverage update attempt",
			"actor_org", actor.OrganizationID, "coverage_id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.not_found"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "common.form_read_error"))
		return
	}

	branchID, err := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	if err != nil || branchID <= 0 {
		branchID = existingCov.BranchID
	}

	var targetBranch *org.Branch
	if h.orgSvc != nil {
		branch, err := h.orgSvc.GetBranch(ctx, branchID)
		if err != nil || branch.OrganizationID != actor.OrganizationID {
			h.log.WarnContext(ctx, "cross-tenant branch coverage update attempt",
				"actor_org", actor.OrganizationID, "target_branch", branchID)
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.branch_not_yours"))
			return
		}
		targetBranch = branch
	}

	dayOfWeek, err := strconv.Atoi(r.PostFormValue("day_of_week"))
	if err != nil || dayOfWeek < 0 || dayOfWeek > 6 {
		dayOfWeek = existingCov.DayOfWeek
	}

	var govID *int64
	if gID, err := strconv.ParseInt(r.PostFormValue("governorate_id"), 10, 64); err == nil && gID > 0 {
		govID = &gID
	} else {
		govID = existingCov.GovernorateID
	}

	var cityID *int64
	var selectedCity *platformadmin.City
	if cID, err := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64); err == nil && cID > 0 {
		cityID = &cID
		if h.adminSvc != nil {
			selectedCity, _ = h.adminSvc.GetCity(ctx, cID)
			if selectedCity != nil && selectedCity.GovernorateID != nil {
				govID = selectedCity.GovernorateID
			}
		}
	} else {
		cityID = existingCov.CityID
	}

	// The radius follows the city, not the form. A vendor editing a coverage
	// row is changing which city, which day and what hours; the extent of the
	// place they picked is not theirs to type. See migration 167.
	distanceMeters := existingCov.DistanceMeters
	if selectedCity != nil {
		distanceMeters = selectedCity.NormalizedRadius()
	}
	if distanceMeters <= 0 {
		distanceMeters = platformadmin.DefaultCoverageRadiusMeters
	}

	var latVal, lngVal *float64
	if selectedCity != nil {
		lat := selectedCity.Latitude
		lon := selectedCity.Longitude
		latVal = &lat
		lngVal = &lon
	} else {
		if latStr := r.PostFormValue("latitude"); latStr != "" {
			if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
				latVal = &lat
			}
		}
		if lngStr := r.PostFormValue("longitude"); lngStr != "" {
			if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
				lngVal = &lng
			}
		}
		if latVal == nil && targetBranch != nil && targetBranch.Latitude != nil {
			latVal = targetBranch.Latitude
		} else if latVal == nil {
			latVal = existingCov.Latitude
		}
		if lngVal == nil && targetBranch != nil && targetBranch.Longitude != nil {
			lngVal = targetBranch.Longitude
		} else if lngVal == nil {
			lngVal = existingCov.Longitude
		}
	}

	address := r.PostFormValue("address")
	if address == "" && selectedCity != nil {
		address = selectedCity.Name.Get("ar")
	} else if address == "" && targetBranch != nil && targetBranch.Address != "" {
		address = targetBranch.Address
	} else if address == "" {
		address = existingCov.Address
	}

	isActive := existingCov.IsActive
	if activeStr := r.PostFormValue("is_active"); activeStr != "" {
		isActive = activeStr == "true" || activeStr == "on" || activeStr == "1"
	}

	var fromTime, toTime *string
	if fStr := strings.TrimSpace(r.PostFormValue("coverage_from")); fStr != "" {
		fromTime = &fStr
	} else {
		fromTime = existingCov.CoverageFrom
	}
	if tStr := strings.TrimSpace(r.PostFormValue("coverage_to")); tStr != "" {
		toTime = &tStr
	} else {
		toTime = existingCov.CoverageTo
	}

	cov := workflow.WeeklyCoverage{
		ID:             id,
		OrganizationID: actor.OrganizationID,
		BranchID:       branchID,
		GovernorateID:  govID,
		CityID:         cityID,
		DayOfWeek:      dayOfWeek,
		CoverageFrom:   fromTime,
		CoverageTo:     toTime,
		Address:        address,
		Latitude:       latVal,
		Longitude:      lngVal,
		DistanceMeters: distanceMeters,
		IsActive:       isActive,
	}

	if err := h.wfSvc.UpdateWeeklyCoverage(ctx, &cov); err != nil {
		h.log.ErrorContext(ctx, "update weekly coverage", "error", err, "org", actor.OrganizationID, "id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.update_error_prefix")+err.Error())
		return
	}

	returnPath := "/vendor/coverage"
	if retBranch := r.PostFormValue("branch_id"); retBranch != "" && retBranch != "all" {
		returnPath = fmt.Sprintf("/vendor/coverage?branch_id=%s", url.QueryEscape(retBranch))
	}
	h.redirectWithNotice(w, r, returnPath, "success", i18n.T(lang, "vendor.coverage.updated_success"))
}

// VendorCoverageDeleteSubmit deletes a weekly coverage record.
func (h *UIHandler) VendorCoverageDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	}
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("coverage_id"), 10, 64)
	}
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.invalid_id"))
		return
	}

	existingCov, err := h.wfSvc.GetWeeklyCoverage(ctx, id)
	if err != nil || existingCov == nil || existingCov.OrganizationID != actor.OrganizationID {
		h.log.WarnContext(ctx, "cross-tenant coverage delete attempt",
			"actor_org", actor.OrganizationID, "coverage_id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.not_found"))
		return
	}

	if err := h.wfSvc.DeleteWeeklyCoverage(ctx, id); err != nil {
		h.log.ErrorContext(ctx, "delete weekly coverage", "error", err, "org", actor.OrganizationID, "id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.delete_error"))
		return
	}

	delReturnPath := "/vendor/coverage"
	if retBranch := r.PostFormValue("return_branch"); retBranch != "" && retBranch != "all" {
		delReturnPath = fmt.Sprintf("/vendor/coverage?branch_id=%s", url.QueryEscape(retBranch))
	}
	h.redirectWithNotice(w, r, delReturnPath, "success", i18n.T(lang, "vendor.coverage.deleted_success"))
}

// VendorCoverageToggleSubmit toggles the active state of a weekly coverage record.
func (h *UIHandler) VendorCoverageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	}
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("coverage_id"), 10, 64)
	}
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.invalid_id"))
		return
	}

	existingCov, err := h.wfSvc.GetWeeklyCoverage(ctx, id)
	if err != nil || existingCov == nil || existingCov.OrganizationID != actor.OrganizationID {
		h.log.WarnContext(ctx, "cross-tenant coverage toggle attempt",
			"actor_org", actor.OrganizationID, "coverage_id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.not_found"))
		return
	}

	newActive := !existingCov.IsActive
	if err := h.wfSvc.ToggleWeeklyCoverage(ctx, id, newActive); err != nil {
		h.log.ErrorContext(ctx, "toggle weekly coverage", "error", err, "org", actor.OrganizationID, "id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.toggle_error"))
		return
	}

	stateLabel := i18n.T(lang, "vendor.coverage.state_enabled")
	if !newActive {
		stateLabel = i18n.T(lang, "vendor.coverage.state_disabled")
	}
	togReturnPath := "/vendor/coverage"
	if retBranch := r.PostFormValue("return_branch"); retBranch != "" && retBranch != "all" {
		togReturnPath = fmt.Sprintf("/vendor/coverage?branch_id=%s", url.QueryEscape(retBranch))
	}
	h.redirectWithNotice(w, r, togReturnPath, "success", fmt.Sprintf(i18n.T(lang, "vendor.coverage.toggle_success"), stateLabel))
}

// VendorCoverageDeleteAllSubmit wipes all weekly coverages belonging to the authenticated vendor organization.
func (h *UIHandler) VendorCoverageDeleteAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	if h.wfSvc != nil {
		if err := h.wfSvc.DeleteAllCoverageForOrganization(ctx, actor.OrganizationID); err != nil {
			h.log.ErrorContext(ctx, "delete all weekly coverages failed", "error", err, "org", actor.OrganizationID)
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.delete_error"))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success", "تم حذف جميع نطاقات التغطية الخاصة بك بنجاح.")
}
