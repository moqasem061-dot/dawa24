package pages

import (
	"encoding/json"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
)

type VendorCoverageData struct {
	Coverages           []*workflow.CoverageView
	Branches            []*org.Branch
	Bands               []*org.DeliveryBand
	Governorates        []*platformadmin.Governorate
	Cities              []*platformadmin.City
	CoverageUnavailable bool
	NoticeType          string
	NoticeMessage       string
}

// cityClientItem is what the selector needs about one city.
type cityClientItem struct {
	ID        int64   `json:"id"`
	GovID     int64   `json:"gov_id"`
	NameAR    string  `json:"name_ar"`
	NameEN    string  `json:"name_en"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	IsCapital bool    `json:"is_capital"`
	RadiusM   int     `json:"radius_m"`
}

func citiesToJSON(cities []*platformadmin.City) string {
	var list []cityClientItem
	for _, c := range cities {
		var gID int64
		if c.GovernorateID != nil {
			gID = *c.GovernorateID
		}
		list = append(list, cityClientItem{
			ID:        c.ID,
			GovID:     gID,
			NameAR:    c.Name.Get("ar"),
			NameEN:    c.Name.Get("en"),
			Lat:       c.Latitude,
			Lon:       c.Longitude,
			IsCapital: c.IsCapital,
			RadiusM:   c.NormalizedRadius(),
		})
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// coverageClientItem is what the Alpine table and pagination need about one coverage row.
type coverageClientItem struct {
	ID              int64    `json:"id"`
	BranchID        int64    `json:"branch_id"`
	BranchName      string   `json:"branch_name"`
	GovernorateID   int64    `json:"governorate_id"`
	GovernorateName string   `json:"governorate_name"`
	CityID          int64    `json:"city_id"`
	CityName        string   `json:"city_name"`
	Address         string   `json:"address"`
	DayOfWeek       int      `json:"day_of_week"`
	DayNameAr       string   `json:"day_name_ar"`
	DayBadgeClass   string   `json:"day_badge_class"`
	DistanceMeters  int      `json:"distance_meters"`
	DistanceKM      string   `json:"distance_km"`
	CoverageFrom    string   `json:"coverage_from"`
	CoverageTo      string   `json:"coverage_to"`
	CoverageWindow  string   `json:"coverage_window"`
	Latitude        *float64 `json:"latitude"`
	Longitude       *float64 `json:"longitude"`
	LatLngStr       string   `json:"lat_lng_str"`
	MapURL          string   `json:"map_url"`
	IsActive        bool     `json:"is_active"`
}

func coveragesToJSON(coverages []*workflow.CoverageView) string {
	list := make([]coverageClientItem, 0, len(coverages))
	for _, c := range coverages {
		if c == nil {
			continue
		}
		var gID int64
		if c.GovernorateID != nil {
			gID = *c.GovernorateID
		}
		var cID int64
		if c.CityID != nil {
			cID = *c.CityID
		}
		govName := c.GovernorateNameAr
		if govName == "" {
			govName = c.GovernorateName
		}
		if govName == "" {
			govName = "مصر"
		}
		cityName := c.CityNameAr
		if cityName == "" {
			cityName = c.CityName
		}
		var covFrom, covTo string
		if c.CoverageFrom != nil {
			covFrom = *c.CoverageFrom
		}
		if c.CoverageTo != nil {
			covTo = *c.CoverageTo
		}
		var latLngStr, mapURL string
		if c.Latitude != nil && c.Longitude != nil {
			latLngStr = fmt.Sprintf("%.4f, %.4f", *c.Latitude, *c.Longitude)
			mapURL = fmt.Sprintf("https://www.google.com/maps?q=%f,%f", *c.Latitude, *c.Longitude)
		}
		list = append(list, coverageClientItem{
			ID:              c.ID,
			BranchID:        c.BranchID,
			BranchName:      c.BranchName,
			GovernorateID:   gID,
			GovernorateName: govName,
			CityID:          cID,
			CityName:        cityName,
			Address:         c.Address,
			DayOfWeek:       c.DayOfWeek,
			DayNameAr:       dayNameArabic(c.DayOfWeek),
			DayBadgeClass:   dayBadgeClass(c.DayOfWeek),
			DistanceMeters:  c.DistanceMeters,
			DistanceKM:      formatDistanceKM(c.DistanceMeters),
			CoverageFrom:    covFrom,
			CoverageTo:      covTo,
			CoverageWindow:  FormatCoverageWindow(c.CoverageFrom, c.CoverageTo),
			Latitude:        c.Latitude,
			Longitude:       c.Longitude,
			LatLngStr:       latLngStr,
			MapURL:          mapURL,
			IsActive:        c.IsActive,
		})
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func dayNameArabic(day int) string {
	switch day {
	case 0:
		return "الأحد"
	case 1:
		return "الاثنين"
	case 2:
		return "الثلاثاء"
	case 3:
		return "الأربعاء"
	case 4:
		return "الخميس"
	case 5:
		return "الجمعة"
	case 6:
		return "السبت"
	default:
		return fmt.Sprintf("يوم %d", day)
	}
}

func dayBadgeClass(day int) string {
	switch day {
	case 0:
		return "badge-primary"
	case 1:
		return "badge-sky"
	case 2:
		return "badge-indigo"
	case 3:
		return "badge-violet"
	case 4:
		return "badge-emerald"
	case 5:
		return "badge-amber"
	case 6:
		return "badge-rose"
	default:
		return "badge-secondary"
	}
}

func countActiveDays(coverages []*workflow.CoverageView) int {
	days := make(map[int]bool)
	for _, c := range coverages {
		if c.IsActive {
			days[c.DayOfWeek] = true
		}
	}
	return len(days)
}

func countCoveredGovernorates(coverages []*workflow.CoverageView) int {
	govs := make(map[string]bool)
	for _, c := range coverages {
		if c.IsActive {
			if c.GovernorateNameAr != "" {
				govs[c.GovernorateNameAr] = true
			} else if c.GovernorateName != "" {
				govs[c.GovernorateName] = true
			}
		}
	}
	return len(govs)
}

func countVendorCoveredCities(coverages []*workflow.CoverageView) int {
	cities := make(map[string]bool)
	for _, c := range coverages {
		if c.IsActive && c.CityID != nil {
			key := fmt.Sprintf("%d_%d", c.DayOfWeek, *c.CityID)
			cities[key] = true
		}
	}
	return len(cities)
}

func formatDistanceKM(meters int) string {
	if meters >= 1000 {
		return fmt.Sprintf("%.1f كم (%d م)", float64(meters)/1000.0, meters)
	}
	return fmt.Sprintf("%d متر", meters)
}
