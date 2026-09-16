package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// VendorCoverageCreateSubmit processes creation of weekly coverage for multiple days and multiple cities.
func (h *UIHandler) VendorCoverageCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "common.form_read_error"))
		return
	}

	branchID, err := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	if err != nil || branchID <= 0 {
		// Fallback to first branch of organization
		if h.orgSvc != nil {
			branches, bErr := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
			if bErr == nil && len(branches) > 0 {
				branchID = branches[0].ID
			}
		}
	}

	if branchID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.branch_required"))
		return
	}

	var targetBranch *org.Branch
	if h.orgSvc != nil {
		branch, err := h.orgSvc.GetBranch(ctx, branchID)
		if err != nil || branch.OrganizationID != actor.OrganizationID {
			h.log.WarnContext(ctx, "cross-tenant branch coverage creation attempt",
				"actor_org", actor.OrganizationID, "target_branch", branchID)
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.branch_not_yours"))
			return
		}
		targetBranch = branch
	}

	// 1. Parse Days of Week (supports multi-select or 'all')
	daysForm := r.PostForm["days_of_week"]
	if len(daysForm) == 0 {
		dayStr := strings.TrimSpace(r.PostFormValue("day_of_week"))
		if dayStr != "" {
			daysForm = []string{dayStr}
		}
	}

	applyAllDays := r.PostFormValue("apply_to_all_days") == "true" || r.PostFormValue("apply_to_all_days") == "on" || r.PostFormValue("apply_to_all_days") == "1"
	for _, d := range daysForm {
		if d == "all" || d == "-1" {
			applyAllDays = true
			break
		}
	}

	var daysToCreate []int
	if applyAllDays {
		daysToCreate = []int{0, 1, 2, 3, 4, 5, 6}
	} else {
		for _, d := range daysForm {
			dayNum, err := strconv.Atoi(strings.TrimSpace(d))
			if err == nil && dayNum >= 0 && dayNum <= 6 {
				// avoid duplicates
				found := false
				for _, existing := range daysToCreate {
					if existing == dayNum {
						found = true
						break
					}
				}
				if !found {
					daysToCreate = append(daysToCreate, dayNum)
				}
			}
		}
	}

	if len(daysToCreate) == 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.day_required"))
		return
	}

	// 2. Parse Governorate & Cities
	var govID *int64
	if gID, err := strconv.ParseInt(r.PostFormValue("governorate_id"), 10, 64); err == nil && gID > 0 {
		govID = &gID
	}

	var targetCities []*platformadmin.City
	allCitiesInGov := r.PostFormValue("all_cities_in_gov") == "true" || r.PostFormValue("all_cities_in_gov") == "on" || r.PostFormValue("all_cities_in_gov") == "1"

	if allCitiesInGov && govID != nil && h.adminSvc != nil {
		citiesInGov, err := h.adminSvc.ListCitiesByGovernorate(ctx, *govID)
		if err == nil && len(citiesInGov) > 0 {
			targetCities = citiesInGov
		}
	}

	if len(targetCities) == 0 {
		cityIDsForm := r.PostForm["city_ids"]
		if len(cityIDsForm) == 0 {
			cIDStr := strings.TrimSpace(r.PostFormValue("city_id"))
			if cIDStr != "" {
				parts := strings.Split(cIDStr, ",")
				for _, p := range parts {
					if strings.TrimSpace(p) != "" {
						cityIDsForm = append(cityIDsForm, strings.TrimSpace(p))
					}
				}
			}
		}

		if len(cityIDsForm) > 0 && h.adminSvc != nil {
			// One read of the city reference table, indexed by id, rather than
			// a GetCity per selected city. A vendor covering a governorate
			// submits dozens of ids at once, and every one of them was a round
			// trip. The table is 351 rows.
			byID := make(map[int64]*platformadmin.City)
			for _, c := range h.listCities(ctx) {
				if c != nil {
					byID[c.ID] = c
				}
			}
			for _, cidStr := range cityIDsForm {
				cID, err := strconv.ParseInt(strings.TrimSpace(cidStr), 10, 64)
				if err != nil || cID <= 0 {
					continue
				}
				city := byID[cID]
				if city == nil {
					continue
				}
				targetCities = append(targetCities, city)
				if govID == nil && city.GovernorateID != nil {
					govID = city.GovernorateID
				}
			}
		}
	}

	// The vendor no longer states a radius. Coverage is the city's own extent,
	// configured per city and applied automatically when the city is selected —
	// see platform_admin migration 167. What the vendor decides is WHICH cities
	// they cover and on which days, which is the question they can answer.
	//
	// The fallback below is only reached by the single-record path, where no
	// city was selected at all and the coverage is a bare point on the map.
	const pointCoverageMeters = platformadmin.DefaultCoverageRadiusMeters

	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on" || r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == ""
	fromTime := workflow.TimeOfDay(r.PostFormValue("coverage_from"))
	toTime := workflow.TimeOfDay(r.PostFormValue("coverage_to"))

	var newCoverages []*workflow.WeeklyCoverage

	if len(targetCities) > 0 {
		for _, city := range targetCities {
			cityID := city.ID
			cityName := city.Name.Get("ar")
			lat := city.Latitude
			lon := city.Longitude

			// Per-city delivery hours stay the vendor's to set: a distributor
			// really does reach المنصورة in the morning and طنطا after noon.
			// The radius does not — it is a property of the place.
			cityFromStr := strings.TrimSpace(r.PostFormValue(fmt.Sprintf("coverage_from_%d", city.ID)))
			cityToStr := strings.TrimSpace(r.PostFormValue(fmt.Sprintf("coverage_to_%d", city.ID)))

			cFromTime := fromTime
			if cityFromStr != "" {
				cFromTime = workflow.TimeOfDay(cityFromStr)
			}
			cToTime := toTime
			if cityToStr != "" {
				cToTime = workflow.TimeOfDay(cityToStr)
			}
			cDistance := city.NormalizedRadius()

			for _, day := range daysToCreate {
				newCoverages = append(newCoverages, &workflow.WeeklyCoverage{
					OrganizationID: actor.OrganizationID,
					BranchID:       branchID,
					GovernorateID:  city.GovernorateID,
					CityID:         &cityID,
					DayOfWeek:      day,
					CoverageFrom:   cFromTime,
					CoverageTo:     cToTime,
					Address:        cityName,
					Latitude:       &lat,
					Longitude:      &lon,
					DistanceMeters: cDistance,
					IsActive:       isActive,
				})
			}
		}
	} else {
		// Single record fallback
		var latVal, lngVal *float64
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
		}
		if lngVal == nil && targetBranch != nil && targetBranch.Longitude != nil {
			lngVal = targetBranch.Longitude
		}

		address := r.PostFormValue("address")
		if address == "" && targetBranch != nil && targetBranch.Address != "" {
			address = targetBranch.Address
		}

		for _, day := range daysToCreate {
			newCoverages = append(newCoverages, &workflow.WeeklyCoverage{
				OrganizationID: actor.OrganizationID,
				BranchID:       branchID,
				GovernorateID:  govID,
				DayOfWeek:      day,
				CoverageFrom:   fromTime,
				CoverageTo:     toTime,
				Address:        address,
				Latitude:       latVal,
				Longitude:      lngVal,
				DistanceMeters: pointCoverageMeters,
				IsActive:       isActive,
			})
		}
	}

	if len(newCoverages) == 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.location_required"))
		return
	}

	if h.wfSvc != nil {
		if err := h.wfSvc.CreateBatchWeeklyCoverage(ctx, newCoverages); err != nil {
			h.log.ErrorContext(ctx, "create batch weekly coverage failed", "error", err, "count", len(newCoverages))
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "vendor.coverage.save_error_prefix")+err.Error())
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success",
		fmt.Sprintf(i18n.T(lang, "vendor.coverage.created_summary"), len(newCoverages)))
}
