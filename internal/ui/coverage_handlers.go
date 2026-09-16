package ui

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorCoveragePage renders the weekly geographic coverage grid, modal forms, and distance delivery tiers.
func (h *UIHandler) VendorCoveragePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	if actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/onboarding/pending", http.StatusSeeOther)
		return
	}

	data := pages.VendorCoverageData{
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMessage: r.URL.Query().Get("msg"),
	}

	if h.wfSvc != nil {
		coverages, err := h.wfSvc.ListCoverageForOrganization(ctx, actor.OrganizationID)
		if err != nil {
			h.log.ErrorContext(ctx, "list weekly coverage", "error", err, "org", actor.OrganizationID)
			data.CoverageUnavailable = true
		} else {
			data.Coverages = coverages
		}
	}

	if h.orgSvc != nil {
		branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
		if err != nil {
			h.log.WarnContext(ctx, "list vendor branches for coverage", "error", err, "org", actor.OrganizationID)
		} else {
			data.Branches = branches
		}

		bands, err := h.orgSvc.GetDeliveryBands(ctx, actor.OrganizationID)
		if err != nil {
			h.log.WarnContext(ctx, "list delivery bands", "error", err, "org", actor.OrganizationID)
		} else {
			data.Bands = bands
		}
	}

	if h.adminSvc != nil {
		govs, err := h.adminSvc.ListGovernorates(ctx, 1)
		if err != nil {
			h.log.WarnContext(ctx, "list governorates for coverage", "error", err)
		} else {
			data.Governorates = govs
		}

		cities, err := h.adminSvc.ListCities(ctx, 1)
		if err != nil {
			h.log.WarnContext(ctx, "list cities for coverage", "error", err)
		} else {
			data.Cities = cities
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorCoveragePage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor coverage", "error", err)
		h.renderError(w, r, err)
	}
}

// VendorBranchCoveragePage redirects branch-specific coverage view to the main coverage console.
func (h *UIHandler) VendorBranchCoveragePage(w http.ResponseWriter, r *http.Request) {
	bID := chi.URLParam(r, "branchID")
	if bID != "" {
		http.Redirect(w, r, "/vendor/coverage?branch_id="+url.QueryEscape(bID), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/vendor/coverage", http.StatusSeeOther)
}

// APIGovernorateCitiesJSON returns all cities belonging to a governorate as JSON.
func (h *UIHandler) APIGovernorateCitiesJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	govID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || govID <= 0 {
		http.Error(w, `{"error":"invalid governorate id"}`, http.StatusBadRequest)
		return
	}

	if h.adminSvc == nil {
		http.Error(w, `{"error":"admin service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	cities, err := h.adminSvc.ListCitiesByGovernorate(ctx, govID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list cities by governorate", "error", err, "gov_id", govID)
		http.Error(w, `{"error":"failed to query cities"}`, http.StatusInternalServerError)
		return
	}

	type cityResponse struct {
		ID        int64   `json:"id"`
		GovID     int64   `json:"gov_id"`
		NameAR    string  `json:"name_ar"`
		NameEN    string  `json:"name_en"`
		Lat       float64 `json:"lat"`
		Lon       float64 `json:"lon"`
		IsCapital bool    `json:"is_capital"`
		Radius    int     `json:"radius,omitempty"`
	}

	resp := make([]cityResponse, 0, len(cities))
	for _, c := range cities {
		resp = append(resp, cityResponse{
			ID:        c.ID,
			GovID:     govID,
			NameAR:    c.Name.Get("ar"),
			NameEN:    c.Name.Get("en"),
			Lat:       c.Latitude,
			Lon:       c.Longitude,
			IsCapital: c.IsCapital,
			Radius:    c.NormalizedRadius(),
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
