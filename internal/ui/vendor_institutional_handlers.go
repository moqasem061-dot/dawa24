package ui

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorPharmacyCoveragePage renders which pharmacies fall inside this vendor's branch coverage schedules.
func (h *UIHandler) VendorPharmacyCoveragePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/pharmacy-coverage", http.StatusSeeOther)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	filterDay := strings.TrimSpace(r.URL.Query().Get("day"))
	filterBranch := strings.TrimSpace(r.URL.Query().Get("branch"))
	filterCity := strings.TrimSpace(r.URL.Query().Get("city"))
	filterType := strings.TrimSpace(r.URL.Query().Get("type"))

	// 1. Fetch vendor's active weekly coverage windows
	var coverages []*workflow.CoverageView
	if h.wfSvc != nil {
		covs, err := h.wfSvc.ListCoverageForOrganization(ctx, actor.OrganizationID)
		if err == nil {
			for _, c := range covs {
				if c != nil && c.IsActive {
					coverages = append(coverages, c)
				}
			}
		}
	}

	// 2. Fetch all approved customer and vendor organizations (excluding self)
	var organizations []*org.Organization
	if h.orgSvc != nil {
		status := org.StatusApproved
		organizations, _ = h.orgSvc.ListOrganizations(ctx, nil, &status, 1000, 0)
	}

	// 3. Load cities map
	cityNames := make(map[int64]string)
	cList := h.listCities(ctx)
	for _, c := range cList {
		if c != nil {
			name := c.Name.Get("ar")
			if name == "" {
				name = c.Name.Get("en")
			}
			cityNames[c.ID] = name
		}
	}

	todayWeekday := int(time.Now().Weekday())
	dayNamesList := []string{
		i18n.T(lang, "day.sunday"),
		i18n.T(lang, "day.monday"),
		i18n.T(lang, "day.tuesday"),
		i18n.T(lang, "day.wednesday"),
		i18n.T(lang, "day.thursday"),
		i18n.T(lang, "day.friday"),
		i18n.T(lang, "day.saturday"),
	}

	var items []pages.CoveredPharmacyItem
	seenCities := make(map[string]bool)
	seenBranches := make(map[string]bool)
	coveredTodayCount := 0

	for _, pharm := range organizations {
		if pharm == nil || pharm.ID == actor.OrganizationID {
			continue
		}

		isVendor := string(pharm.Type) == "vendor" || string(pharm.Type) == "supplier" || string(pharm.Type) == "distributor" || string(pharm.Type) == "company"
		orgTypeStr := "customer"
		if isVendor {
			orgTypeStr = "vendor"
		}
		if filterType == "customer" && isVendor {
			continue
		}
		if filterType == "vendor" && !isVendor {
			continue
		}

		pharmName := orgName(pharm)
		if pharmName == "" {
			pharmName = pharm.LegalName
		}
		tradeName := pharm.TradeName.Get("ar")
		if tradeName == "" {
			tradeName = pharm.TradeName.Get("en")
		}

		var branches []*org.Branch
		if h.orgSvc != nil {
			branches, _ = h.orgSvc.ListBranches(ctx, pharm.ID)
		}

		for _, pb := range branches {
			if pb == nil || pb.Status != "active" {
				continue
			}

			pbBranchName := pb.Name["ar"]
			if pbBranchName == "" {
				pbBranchName = pb.Name["en"]
			}
			if pbBranchName == "" {
				pbBranchName = i18n.T(lang, "customer.branch.default_name")
			}

			cityName := ""
			if pb.CityID != nil {
				if cn, ok := cityNames[*pb.CityID]; ok {
					cityName = cn
				}
			}

			// Match against vendor coverage windows
			matched := false
			minDist := math.MaxInt32
			daysMap := make(map[int]bool)
			var matchedBranchName string
			var matchedBranchID int64
			var timeWindow string
			matchReason := ""

			for _, cov := range coverages {
				isMatch := false
				dist := -1

				hasCoords := cov.Latitude != nil && cov.Longitude != nil && pb.Latitude != nil && pb.Longitude != nil
				// 1. Match by Coordinates / Distance radius
				if hasCoords {
					dist = haversineCoverageDistance(*cov.Latitude, *cov.Longitude, *pb.Latitude, *pb.Longitude)
					maxDist := cov.DistanceMeters
					if maxDist <= 0 {
						maxDist = 3000 // default 3km
					}
					if dist <= maxDist {
						isMatch = true
						if matchReason == "" {
							if dist < 1000 {
								matchReason = fmt.Sprintf(i18n.T(lang, "vendor.coverage.within_range_meters"), dist)
							} else {
								matchReason = fmt.Sprintf(i18n.T(lang, "vendor.coverage.within_range_km"), float64(dist)/1000.0)
							}
						}
					}
				}

				// 2. Match by City ID fallback only if coordinates are absent
				if !hasCoords && cov.CityID != nil && pb.CityID != nil && *cov.CityID == *pb.CityID {
					isMatch = true
					if matchReason == "" {
						matchReason = i18n.T(lang, "vendor.coverage.within_distribution_city")
					}
				}

				if isMatch {
					matched = true
					daysMap[cov.DayOfWeek] = true
					if dist >= 0 && dist < minDist {
						minDist = dist
					}
					if matchedBranchName == "" {
						matchedBranchName = cov.BranchName
						matchedBranchID = cov.BranchID
					}
					if timeWindow == "" && ((cov.CoverageFrom != nil && *cov.CoverageFrom != "") || (cov.CoverageTo != nil && *cov.CoverageTo != "")) {
						timeWindow = pages.FormatCoverageWindow(cov.CoverageFrom, cov.CoverageTo)
					}
				}
			}

			if !matched {
				continue
			}

			var coveredDays []int
			var coveredDaysLabels []string
			isCoveredToday := false

			for d := 0; d <= 6; d++ {
				if daysMap[d] {
					coveredDays = append(coveredDays, d)
					if d < len(dayNamesList) {
						coveredDaysLabels = append(coveredDaysLabels, dayNamesList[d])
					}
					if d == todayWeekday {
						isCoveredToday = true
					}
				}
			}

			if isCoveredToday {
				coveredTodayCount++
			}

			if cityName != "" {
				seenCities[cityName] = true
			}
			if matchedBranchName != "" {
				seenBranches[matchedBranchName] = true
			}

			distanceKM := 0.0
			distanceMetersVal := 0
			if minDist != math.MaxInt32 {
				distanceMetersVal = minDist
				distanceKM = math.Round((float64(minDist)/1000.0)*100) / 100
			}

			item := pages.CoveredPharmacyItem{
				PharmacyID:         pharm.ID,
				PharmacyName:       pharmName,
				PharmacyTradeName:  tradeName,
				OrgType:            orgTypeStr,
				IsVendor:           isVendor,
				BranchID:           pb.ID,
				BranchName:         pbBranchName,
				Address:            pb.Address,
				Phone:              pb.Phone,
				CityID:             pb.CityID,
				CityName:           cityName,
				CoveringBranchID:   matchedBranchID,
				CoveringBranchName: matchedBranchName,
				DistanceMeters:     distanceMetersVal,
				DistanceKM:         distanceKM,
				CoveredDays:        coveredDays,
				CoveredDaysLabels:  coveredDaysLabels,
				TimeWindow:         timeWindow,
				IsCoveredToday:     isCoveredToday,
				MatchReason:        matchReason,
			}

			// Apply Search Filter
			if search != "" {
				qLow := strings.ToLower(search)
				if !strings.Contains(strings.ToLower(item.PharmacyName), qLow) &&
					!strings.Contains(strings.ToLower(item.PharmacyTradeName), qLow) &&
					!strings.Contains(strings.ToLower(item.BranchName), qLow) &&
					!strings.Contains(strings.ToLower(item.Address), qLow) &&
					!strings.Contains(strings.ToLower(item.Phone), qLow) &&
					!strings.Contains(strings.ToLower(item.CityName), qLow) {
					continue
				}
			}

			// Apply Day Filter
			if filterDay != "" {
				if filterDay == "today" {
					if !isCoveredToday {
						continue
					}
				} else if dInt, err := strconv.Atoi(filterDay); err == nil {
					if !daysMap[dInt] {
						continue
					}
				}
			}

			// Apply Branch Filter
			if filterBranch != "" && matchedBranchName != filterBranch {
				continue
			}

			// Apply City Filter
			if filterCity != "" && cityName != filterCity {
				continue
			}

			items = append(items, item)
		}
	}

	var coveredCitiesList []string
	for c := range seenCities {
		coveredCitiesList = append(coveredCitiesList, c)
	}
	var coveredBranchesList []string
	for b := range seenBranches {
		coveredBranchesList = append(coveredBranchesList, b)
	}

	totalPharmacies := 0
	totalVendors := 0
	for _, it := range items {
		if it.IsVendor {
			totalVendors++
		} else {
			totalPharmacies++
		}
	}

	data := pages.VendorPharmacyCoverageData{
		Pharmacies:        items,
		TotalPharmacies:   totalPharmacies,
		TotalVendors:      totalVendors,
		TotalFacilities:   len(items),
		CoveredTodayCount: coveredTodayCount,
		CoveredCities:     coveredCitiesList,
		CoveredBranches:   coveredBranchesList,
		FilterDay:         filterDay,
		FilterBranch:      filterBranch,
		FilterCity:        filterCity,
		FilterType:        filterType,
		SearchQuery:       search,
	}

	h.renderPage(ctx, w, "render vendor pharmacy coverage", pages.VendorPharmacyCoveragePage(data, lang, dir))
}

// VendorPharmacyCoverageDetailPage renders single pharmacy coverage detail.
func (h *UIHandler) VendorPharmacyCoverageDetailPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/vendor/pharmacy-coverage?id=%s", idStr), http.StatusSeeOther)
}

func haversineCoverageDistance(lat1, lon1, lat2, lon2 float64) int {
	const earthRadius = 6371000.0 // meters
	dLat := (lat2 - lat1) * (math.Pi / 180.0)
	dLon := (lon2 - lon1) * (math.Pi / 180.0)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*(math.Pi/180.0))*math.Cos(lat2*(math.Pi/180.0))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return int(earthRadius * c)
}
