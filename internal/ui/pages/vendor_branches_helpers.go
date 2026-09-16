package pages

import (
	"encoding/json"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type VendorBranchesData struct {
	Branches           []*org.Branch
	Governorates       []*platformadmin.Governorate
	Cities             []*platformadmin.City
	Employees          []*org.EmployeeView
	InstitutionalWorks []*org.InstitutionalWork
	Page               int
	PerPage            int
	TotalCount         int
}

func (d VendorBranchesData) WorkLabel(key, lang string) string {
	for _, w := range d.InstitutionalWorks {
		if fmt.Sprintf("%d", w.ID) == key || w.Slug == key {
			if title := w.Title.Get(i18n.Lang(lang)); title != "" {
				return title
			}
			if title := w.Title.Get(i18n.AR); title != "" {
				return title
			}
			if title := w.Title.Get(i18n.EN); title != "" {
				return title
			}
		}
	}
	return formatInstitutionalWorkLabel(key)
}

func formatVendorBranchJSON(b *org.Branch, cities []*platformadmin.City) string {
	if b == nil {
		return "{}"
	}
	cityID := int64(0)
	govID := int64(0)
	if b.CityID != nil {
		cityID = *b.CityID
		for _, c := range cities {
			if c != nil && c.ID == cityID && c.GovernorateID != nil {
				govID = *c.GovernorateID
				break
			}
		}
	}
	managerID := int64(0)
	if b.ManagerID != nil {
		managerID = *b.ManagerID
	}
	lat := 30.0444
	if b.Latitude != nil {
		lat = *b.Latitude
	}
	lon := 31.2357
	if b.Longitude != nil {
		lon = *b.Longitude
	}
	m := map[string]any{
		"id":                  b.ID,
		"name_ar":             b.Name.Get("ar"),
		"name_en":             b.Name.Get("en"),
		"code":                b.Code,
		"warehouse_type":      b.WarehouseType,
		"city_id":             cityID,
		"governorate_id":      govID,
		"manager_id":          managerID,
		"address":             b.Address,
		"phone":               b.Phone,
		"capacity_sqm":        b.CapacitySQM,
		"google_maps_url":     b.GoogleMapsURL,
		"has_cold_storage":    b.HasColdStorage,
		"is_main":             b.IsMain,
		"latitude":            lat,
		"longitude":           lon,
		"institutional_works": b.InstitutionalWorks,
	}
	bytes, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func countColdStorageBranches(branches []*org.Branch) int {
	c := 0
	for _, b := range branches {
		if b.HasColdStorage {
			c++
		}
	}
	return c
}

func totalCapacitySQM(branches []*org.Branch) float64 {
	var total float64
	for _, b := range branches {
		total += b.CapacitySQM
	}
	return total
}

func formatWorksSliceJSON(works []string) string {
	if len(works) == 0 {
		return "[]"
	}
	b, err := json.Marshal(works)
	if err != nil {
		return "[]"
	}
	return string(b)
}
