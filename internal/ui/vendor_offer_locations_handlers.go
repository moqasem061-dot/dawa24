package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorOfferLocationsPage renders geographic location coverage management for an offer.
func (h *UIHandler) VendorOfferLocationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 || h.promoSvc == nil {
		http.Redirect(w, r, "/vendor/offers", http.StatusSeeOther)
		return
	}

	offer, err := h.promoSvc.GetSpecialOffer(ctx, id)
	if err != nil || offer == nil || offer.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.not_found"))
		return
	}

	locs, _ := h.promoSvc.ListSpecialOfferLocations(ctx, id)
	govs := h.listGovernorates(ctx)
	cities := h.listCities(ctx)

	cityMap := make(map[int64]*platformadmin.City, len(cities))
	for _, c := range cities {
		if c != nil {
			cityMap[c.ID] = c
		}
	}
	govMap := make(map[int64]*platformadmin.Governorate, len(govs))
	for _, g := range govs {
		if g != nil {
			govMap[g.ID] = g
		}
	}

	var editLoc *promo.SpecialOfferLocation
	var editID int64
	if ep := r.URL.Query().Get("edit"); ep != "" {
		editID, _ = strconv.ParseInt(ep, 10, 64)
	}

	for _, loc := range locs {
		if loc.CityID != nil {
			if c, ok := cityMap[*loc.CityID]; ok && c != nil {
				if loc.CityName == "" {
					loc.CityName = c.Name.Get(i18n.ParseLang(lang))
					if loc.CityName == "" {
						loc.CityName = c.Name.Get("ar")
					}
				}
				if c.GovernorateID != nil {
					loc.GovernorateID = c.GovernorateID
					if g, ok := govMap[*c.GovernorateID]; ok && g != nil {
						loc.GovernorateName = g.Name.Get(i18n.ParseLang(lang))
						if loc.GovernorateName == "" {
							loc.GovernorateName = g.Name.Get("ar")
						}
					}
				}
			}
		}
		if editID > 0 && loc.ID == editID {
			editLoc = loc
		}
	}

	noticeType := r.URL.Query().Get("notice_type")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice")
	}
	noticeMsg := r.URL.Query().Get("notice")
	if r.URL.Query().Get("msg") != "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	data := pages.VendorOfferLocationsData{
		Offer:         offer,
		Locations:     locs,
		Governorates:  govs,
		Cities:        cities,
		EditLocation:  editLoc,
		NoticeType:    noticeType,
		NoticeMessage: noticeMsg,
	}

	h.renderPage(ctx, w, "render vendor offer locations page", pages.VendorOfferLocationsPage(data, lang, dir))
}

// VendorOfferLocationNewSubmit adds or updates a geographic coverage location for an offer.
func (h *UIHandler) VendorOfferLocationNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	offerID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	back := fmt.Sprintf("/vendor/offers/%d/locations", offerID)
	if offerID <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	offer, err := h.promoSvc.GetSpecialOffer(ctx, offerID)
	if err != nil || offer == nil || offer.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.forbidden"))
		return
	}

	locIDVal, _ := strconv.ParseInt(chi.URLParam(r, "locId"), 10, 64)
	if locIDVal <= 0 {
		locIDVal, _ = strconv.ParseInt(r.PostFormValue("loc_id"), 10, 64)
	}

	// 1. Parse Days of Week (1=Saturday ... 7=Friday)
	daysForm := r.PostForm["days_of_week"]
	if len(daysForm) == 0 {
		if dStr := strings.TrimSpace(r.PostFormValue("day_of_week")); dStr != "" {
			daysForm = []string{dStr}
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
		daysToCreate = []int{1, 2, 3, 4, 5, 6, 7}
	} else {
		for _, d := range daysForm {
			dNum, err := strconv.Atoi(strings.TrimSpace(d))
			if err == nil && dNum >= 1 && dNum <= 7 {
				found := false
				for _, existing := range daysToCreate {
					if existing == dNum {
						found = true
						break
					}
				}
				if !found {
					daysToCreate = append(daysToCreate, dNum)
				}
			}
		}
	}
	if len(daysToCreate) == 0 {
		daysToCreate = []int{1}
	}

	// 2. Parse Governorate & Cities
	var govID *int64
	if gID, err := strconv.ParseInt(r.PostFormValue("governorate_id"), 10, 64); err == nil && gID > 0 {
		govID = &gID
	}

	allCities := h.listCities(ctx)
	cityMap := make(map[int64]*platformadmin.City, len(allCities))
	for _, c := range allCities {
		if c != nil {
			cityMap[c.ID] = c
		}
	}

	var targetCities []*platformadmin.City
	allCitiesInGov := r.PostFormValue("all_cities_in_gov") == "true" || r.PostFormValue("all_cities_in_gov") == "on" || r.PostFormValue("all_cities_in_gov") == "1"
	if allCitiesInGov && govID != nil && h.adminSvc != nil {
		if citiesInGov, err := h.adminSvc.ListCitiesByGovernorate(ctx, *govID); err == nil && len(citiesInGov) > 0 {
			targetCities = citiesInGov
		}
	}

	if len(targetCities) == 0 {
		cityIDsForm := r.PostForm["city_ids"]
		if len(cityIDsForm) == 0 {
			if cIDStr := strings.TrimSpace(r.PostFormValue("city_id")); cIDStr != "" {
				parts := strings.Split(cIDStr, ",")
				for _, p := range parts {
					if s := strings.TrimSpace(p); s != "" {
						cityIDsForm = append(cityIDsForm, s)
					}
				}
			}
		}
		for _, cidStr := range cityIDsForm {
			cID, err := strconv.ParseInt(strings.TrimSpace(cidStr), 10, 64)
			if err != nil || cID <= 0 {
				continue
			}
			if c, ok := cityMap[cID]; ok && c != nil {
				targetCities = append(targetCities, c)
			}
		}
	}

	if len(targetCities) == 0 && locIDVal <= 0 {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.offer.city_required"))
		return
	}

	radius, _ := strconv.Atoi(r.PostFormValue("radius"))
	timeFrom := strings.TrimSpace(r.PostFormValue("time_from"))
	timeTo := strings.TrimSpace(r.PostFormValue("time_to"))
	addrAr := strings.TrimSpace(r.PostFormValue("address_ar"))
	addrEn := strings.TrimSpace(r.PostFormValue("address_en"))

	// If editing an existing single location
	if locIDVal > 0 {
		var editCity *platformadmin.City
		if len(targetCities) > 0 {
			editCity = targetCities[0]
		}
		lat, _ := strconv.ParseFloat(r.PostFormValue("latitude"), 64)
		lon, _ := strconv.ParseFloat(r.PostFormValue("longitude"), 64)
		var cIDPtr *int64
		if editCity != nil {
			cIDPtr = &editCity.ID
			if lat == 0 && lon == 0 {
				lat = editCity.Latitude
				lon = editCity.Longitude
			}
			if radius <= 0 {
				radius = editCity.NormalizedRadius()
			}
			if addrAr == "" {
				addrAr = editCity.Name.Get("ar")
				addrEn = editCity.Name.Get("en")
			}
		}
		if radius < platformadmin.MinCoverageRadiusMeters {
			radius = platformadmin.MinCoverageRadiusMeters
		}
		if radius > platformadmin.MaxCoverageRadiusMeters {
			radius = platformadmin.MaxCoverageRadiusMeters
		}
		loc := &promo.SpecialOfferLocation{
			OfferID:     offerID,
			CityID:      cIDPtr,
			AddressAr:   addrAr,
			AddressEn:   addrEn,
			Latitude:    lat,
			Longitude:   lon,
			Radius:      radius,
			DayOfWeek:   daysToCreate[0],
			TimeFrom:    timeFrom,
			TimeTo:      timeTo,
			Status:      "active",
			AdminStatus: "approved",
		}
		_ = h.promoSvc.DeleteSpecialOfferLocation(ctx, locIDVal, offerID, actor.OrganizationID)
		if err := h.promoSvc.AddSpecialOfferLocation(ctx, loc); err != nil {
			h.log.ErrorContext(ctx, "update special offer location", "error", err, "offer_id", offerID)
			h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.offer.location_failed"))
			return
		}
		h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "vendor.offer.location_updated_success"))
		return
	}

	// Bulk create for all (day, city) pairs
	createdCount := 0
	for _, day := range daysToCreate {
		for _, city := range targetCities {
			cityRadius := radius
			if cityRadius <= 0 {
				cityRadius = city.NormalizedRadius()
			}
			if cityRadius < platformadmin.MinCoverageRadiusMeters {
				cityRadius = platformadmin.MinCoverageRadiusMeters
			}
			if cityRadius > platformadmin.MaxCoverageRadiusMeters {
				cityRadius = platformadmin.MaxCoverageRadiusMeters
			}
			cAddrAr := addrAr
			cAddrEn := addrEn
			if cAddrAr == "" {
				cAddrAr = city.Name.Get("ar")
				cAddrEn = city.Name.Get("en")
			}
			cID := city.ID
			loc := &promo.SpecialOfferLocation{
				OfferID:     offerID,
				CityID:      &cID,
				AddressAr:   cAddrAr,
				AddressEn:   cAddrEn,
				Latitude:    city.Latitude,
				Longitude:   city.Longitude,
				Radius:      cityRadius,
				DayOfWeek:   day,
				TimeFrom:    timeFrom,
				TimeTo:      timeTo,
				Status:      "active",
				AdminStatus: "approved",
			}
			if err := h.promoSvc.AddSpecialOfferLocation(ctx, loc); err == nil {
				createdCount++
			} else {
				h.log.WarnContext(ctx, "add special offer location failed in batch", "error", err, "city_id", city.ID, "day", day)
			}
		}
	}

	if createdCount == 0 {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.offer.location_failed"))
		return
	}

	msg := fmt.Sprintf("تمت إضافة نطاقات التغطية بنجاح (%d نطاق تغطية)", createdCount)
	h.redirectWithNotice(w, r, back, "success", msg)
}

// VendorOfferLocationDeleteSubmit deletes a geographic coverage location from an offer.
func (h *UIHandler) VendorOfferLocationDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	offerID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	locID, _ := strconv.ParseInt(chi.URLParam(r, "locId"), 10, 64)
	back := fmt.Sprintf("/vendor/offers/%d/locations", offerID)

	if offerID <= 0 || locID <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.offer.location_not_found"))
		return
	}

	offer, err := h.promoSvc.GetSpecialOffer(ctx, offerID)
	if err != nil || offer == nil || offer.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.forbidden"))
		return
	}

	if err := h.promoSvc.DeleteSpecialOfferLocation(ctx, locID, offerID, actor.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "delete special offer location", "error", err, "offer_id", offerID, "loc_id", locID)
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.offer.location_delete_failed"))
		return
	}

	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "vendor.offer.location_deleted_success"))
}

// VendorOfferDeleteSubmit deletes a special offer.
func (h *UIHandler) VendorOfferDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.not_found"))
		return
	}

	// DeleteSpecialOffer is scoped by organization, so a foreign id deletes
	// nothing — but silence made that indistinguishable from success.
	if err := h.promoSvc.DeleteSpecialOffer(ctx, id, actor.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "delete special offer", "error", err, "offer_id", id)
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.delete_failed"))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/offers", "success", i18n.T(lang, "vendor.offer.deleted_success"))
}
