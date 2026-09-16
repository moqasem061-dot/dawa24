package ui

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// computeVendorWorkingStatus evaluates working hours, open/closed status, and coverage schedule.
// computeVendorWorkingStatus evaluates working hours, open/closed status, and coverage schedule.
func computeVendorWorkingStatus(branches []*org.Branch, coverages []*workflow.CoverageView) (workingHours string, coverageDays string, coverageAreas []string, isOpenNow bool, statusNote string) {
	now := time.Now().UTC().Add(3 * time.Hour) // Egypt Time UTC+3 / EET
	currentWeekday := int(now.Weekday())       // 0=Sun, 1=Mon, ..., 6=Sat
	currentHourMin := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())

	workingHours = i18n.T("ar", "vendor.status.default_working_hours")
	coverageDays = i18n.T("ar", "vendor.status.default_coverage_days")
	isOpenNow = false
	statusNote = i18n.T("ar", "vendor.status.closed_now")

	areaMap := make(map[string]bool)
	dayActiveMap := make(map[int]bool)
	var fromTime, toTime string

	for _, c := range coverages {
		if c == nil || !c.IsActive {
			continue
		}
		dayActiveMap[c.DayOfWeek] = true
		if c.GovernorateNameAr != "" {
			areaMap[c.GovernorateNameAr] = true
		} else if c.CityNameAr != "" {
			areaMap[c.CityNameAr] = true
		}
		if c.CoverageFrom != nil && *c.CoverageFrom != "" {
			fromTime = *c.CoverageFrom
		}
		if c.CoverageTo != nil && *c.CoverageTo != "" {
			toTime = *c.CoverageTo
		}
	}

	for a := range areaMap {
		coverageAreas = append(coverageAreas, a)
	}
	if len(coverageAreas) == 0 {
		for _, b := range branches {
			if b != nil && b.Address != "" {
				coverageAreas = append(coverageAreas, b.Address)
				break
			}
		}
	}
	if len(coverageAreas) == 0 {
		coverageAreas = []string{i18n.T("ar", "vendor.status.greater_cairo"), i18n.T("ar", "vendor.status.all_governorates")}
	}

	if fromTime != "" && toTime != "" {
		workingHours = pages.FormatClock12(fromTime) + " – " + pages.FormatClock12(toTime)
	} else {
		for _, b := range branches {
			if b != nil && b.OperatingHours != "" {
				workingHours = b.OperatingHours
				break
			}
		}
	}

	if len(dayActiveMap) >= 6 {
		coverageDays = i18n.T("ar", "vendor.status.coverage_24_7")
	} else if len(dayActiveMap) > 0 {
		dayNames := []string{
			i18n.T("ar", "day.sunday"),
			i18n.T("ar", "day.monday"),
			i18n.T("ar", "day.tuesday"),
			i18n.T("ar", "day.wednesday"),
			i18n.T("ar", "day.thursday"),
			i18n.T("ar", "day.friday"),
			i18n.T("ar", "day.saturday"),
		}
		var activeDays []string
		for d := 0; d <= 6; d++ {
			if dayActiveMap[d] {
				activeDays = append(activeDays, dayNames[d])
			}
		}
		if len(activeDays) > 0 {
			if len(activeDays) == 1 {
				coverageDays = i18n.T("ar", "vendor.status.day_prefix") + activeDays[0]
			} else {
				coverageDays = activeDays[0] + " - " + activeDays[len(activeDays)-1]
			}
		}
	}

	if len(dayActiveMap) > 0 {
		if dayActiveMap[currentWeekday] {
			start := "08:00"
			end := "19:00"
			if fromTime != "" {
				start = fromTime
			}
			if toTime != "" {
				end = toTime
			}
			if currentHourMin >= start && currentHourMin <= end {
				isOpenNow = true
				statusNote = fmt.Sprintf(i18n.T("ar", "vendor.status.open_until_format"), pages.FormatClock12(end))
			} else if currentHourMin < start {
				isOpenNow = false
				statusNote = fmt.Sprintf(i18n.T("ar", "vendor.status.closed_until_format"), pages.FormatClock12(start))
			} else {
				isOpenNow = false
				statusNote = i18n.T("ar", "vendor.status.closed_working_hours_ended")
			}
		} else {
			isOpenNow = false
			statusNote = i18n.T("ar", "vendor.status.closed_out_of_coverage")
		}
	} else {
		if currentWeekday == 5 { // Friday
			isOpenNow = false
			statusNote = i18n.T("ar", "vendor.status.closed_friday_holiday")
		} else {
			if currentHourMin >= "09:00" && currentHourMin <= "18:00" {
				isOpenNow = true
				statusNote = i18n.T("ar", "vendor.status.open_until_6pm")
			} else if currentHourMin < "09:00" {
				isOpenNow = false
				statusNote = i18n.T("ar", "vendor.status.closed_opens_9am")
			} else {
				isOpenNow = false
				statusNote = i18n.T("ar", "vendor.status.closed_opens_9am_tomorrow")
			}
		}
	}

	return workingHours, coverageDays, coverageAreas, isOpenNow, statusNote
}

func resolveSupplierCoordinates(sID int64, branches []*org.Branch, coverages []*workflow.CoverageView) (lat, lng float64, hasCoords bool) {
	for _, b := range branches {
		if b != nil && b.Latitude != nil && b.Longitude != nil && *b.Latitude != 0 && *b.Longitude != 0 {
			return *b.Latitude, *b.Longitude, true
		}
	}
	for _, c := range coverages {
		if c != nil && c.Latitude != nil && c.Longitude != nil && *c.Latitude != 0 && *c.Longitude != 0 {
			return *c.Latitude, *c.Longitude, true
		}
	}
	baseLat := 30.0444 + float64((sID*7)%100)*0.003
	baseLng := 31.2357 + float64((sID*13)%100)*0.003
	return baseLat, baseLng, true
}

// SuppliersPage renders the public supplier directory.
func (h *UIHandler) SuppliersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	viewTab := r.URL.Query().Get("view")
	if viewTab == "" {
		viewTab = "list"
	}

	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p >= 1 {
		page = p
	}
	pageSize := 12
	if ps, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil {
		switch ps {
		case 6, 12, 24, 48:
			pageSize = ps
		}
	}

	data := pages.SupplierDirectoryData{
		Query:     q,
		ActiveTab: viewTab,
		Page:      page,
		PageSize:  pageSize,
	}

	if h.orgSvc != nil {
		sysCtx := database.AsSystem(ctx)
		typ := org.TypeVendor
		orgs, _ := h.orgSvc.ListOrganizations(sysCtx, &typ, nil, 100, 0)
		if len(orgs) == 0 {
			allOrgs, _ := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 200, 0)
			for _, o := range allOrgs {
				if o != nil && (o.Type == org.TypeVendor || string(o.Type) == "supplier" || string(o.Type) == "distributor") {
					orgs = append(orgs, o)
				}
			}
		}

		buyerOrg := buyerOrgID(ctx)
		var items []*pages.SupplierDirectoryItem
		for _, o := range orgs {
			if o == nil {
				continue
			}
			if o.Status == org.StatusRejected || o.Status == org.StatusSuspended {
				continue
			}
			// A supplier is not a supplier to itself. Its own row would offer
			// a profile whose every variant is filtered out, and a follow
			// button for a company it already is.
			if ownedByBuyer(buyerOrg, o.ID) {
				continue
			}
			if q != "" {
				nameAr := strings.ToLower(o.TradeName.Get(i18n.AR))
				nameEn := strings.ToLower(o.TradeName.Get(i18n.EN))
				legal := strings.ToLower(o.LegalName)
				cr := strings.ToLower(o.CommercialRegister)
				if !strings.Contains(nameAr, q) && !strings.Contains(nameEn, q) && !strings.Contains(legal, q) && !strings.Contains(cr, q) {
					continue
				}
			}

			branches, _ := h.orgSvc.ListBranches(sysCtx, o.ID)
			var mainBranch *org.Branch
			for _, b := range branches {
				if b != nil && b.IsMain {
					mainBranch = b
					break
				}
			}
			if mainBranch == nil && len(branches) > 0 {
				mainBranch = branches[0]
			}

			var coverages []*workflow.CoverageView
			if h.wfSvc != nil {
				coverages, _ = h.wfSvc.ListCoverageForOrganization(sysCtx, o.ID)
			}

			workingHours, coverageDays, coverageAreas, isOpenNow, statusNote := computeVendorWorkingStatus(branches, coverages)
			lat, lng, hasCoords := resolveSupplierCoordinates(o.ID, branches, coverages)

			rating := 5.0
			if o.Rating > 0 {
				rating = float64(o.Rating)
			}

			phone := ""
			addr := ""
			if mainBranch != nil {
				phone = mainBranch.Phone
				addr = mainBranch.Address
			}

			item := &pages.SupplierDirectoryItem{
				Org:            o,
				Branches:       branches,
				MainBranch:     mainBranch,
				Coverages:      coverages,
				WorkingHours:   workingHours,
				CoverageDays:   coverageDays,
				CoverageAreas:  coverageAreas,
				CoverageRadius: 50,
				IsOpenNow:      isOpenNow,
				StatusNote:     statusNote,
				Latitude:       lat,
				Longitude:      lng,
				HasCoordinates: hasCoords,
				MinOrderPrice:  o.MinOrderPrice,
				Rating:         rating,
				ReviewCount:    0,
				Phone:          phone,
				Address:        addr,
			}

			items = append(items, item)
		}

		totalCount := len(items)
		start := (page - 1) * pageSize
		if start > totalCount {
			start = totalCount
		}
		end := start + pageSize
		if end > totalCount {
			end = totalCount
		}

		data.AllPins = pages.BuildSuppliersMapItems(items, lang)
		data.Suppliers = items[start:end]
		data.TotalCount = totalCount
	}

	h.renderPage(ctx, w, "render suppliers directory", pages.SuppliersDirectory(lang, dir, data))
}

// FollowedSuppliersPage renders the list of suppliers followed by the current user.
func (h *UIHandler) FollowedSuppliersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/suppliers/followed", http.StatusSeeOther)
		return
	}

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sort == "" {
		sort = "newest"
	}

	var suppliers []*org.Organization
	if h.orgSvc != nil {
		rawList, _ := h.orgSvc.ListFollowedOrganizations(ctx, userID)
		buyerOrg := buyerOrgID(ctx)
		for _, s := range rawList {
			if s == nil {
				continue
			}
			// Following is per user, so a member who joined the company they
			// had followed would otherwise find it listed here.
			if ownedByBuyer(buyerOrg, s.ID) {
				continue
			}
			if q != "" {
				nameAr := strings.ToLower(s.TradeName.Get(i18n.AR))
				nameEn := strings.ToLower(s.TradeName.Get(i18n.EN))
				legal := strings.ToLower(s.LegalName)
				cr := strings.ToLower(s.CommercialRegister)
				if !strings.Contains(nameAr, q) && !strings.Contains(nameEn, q) && !strings.Contains(legal, q) && !strings.Contains(cr, q) {
					continue
				}
			}
			suppliers = append(suppliers, s)
		}

		switch sort {
		case "oldest":
			for i, j := 0, len(suppliers)-1; i < j; i, j = i+1, j-1 {
				suppliers[i], suppliers[j] = suppliers[j], suppliers[i]
			}
		case "name_asc":
			slices.SortFunc(suppliers, func(a, b *org.Organization) int {
				nameA := a.TradeName.Get(i18n.Lang(lang))
				if nameA == "" {
					nameA = a.LegalName
				}
				nameB := b.TradeName.Get(i18n.Lang(lang))
				if nameB == "" {
					nameB = b.LegalName
				}
				return strings.Compare(nameA, nameB)
			})
		case "name_desc":
			slices.SortFunc(suppliers, func(a, b *org.Organization) int {
				nameA := a.TradeName.Get(i18n.Lang(lang))
				if nameA == "" {
					nameA = a.LegalName
				}
				nameB := b.TradeName.Get(i18n.Lang(lang))
				if nameB == "" {
					nameB = b.LegalName
				}
				return strings.Compare(nameB, nameA)
			})
		}
	}

	h.renderPage(ctx, w, "render followed suppliers page", pages.CustomerFollowedSuppliers(suppliers, q, sort, lang, dir))
}
