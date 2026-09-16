package ui

import (
	"context"
	"strconv"
	"strings"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
)

// The geography the registration form offers and the checks on what comes back.
//
// Split from auth_handlers.go for the 400-line rule.

// listCities loads the Egyptian cities for the registration form's city picker.
// listCities loads the Egyptian cities for the registration form's city picker.
func (h *UIHandler) listCities(ctx context.Context) []*platformadmin.City {
	if h.adminSvc == nil {
		return nil
	}
	countries, err := h.adminSvc.ListCountries(ctx)
	if err != nil || len(countries) == 0 {
		return nil
	}
	var countryID int64
	for _, c := range countries {
		if c.Code == "EG" {
			countryID = c.ID
			break
		}
	}
	if countryID == 0 {
		countryID = countries[0].ID
	}
	cities, _ := h.adminSvc.ListCities(ctx, countryID)
	return cities
}

// listGovernorates loads Egypt's governorates for the registration pickers.
func (h *UIHandler) listGovernorates(ctx context.Context) []*platformadmin.Governorate {
	if h.adminSvc == nil {
		return nil
	}
	countryID := h.egyptCountryID(ctx)
	if countryID == 0 {
		return nil
	}
	govs, err := h.adminSvc.ListGovernorates(ctx, countryID)
	if err != nil {
		h.log.ErrorContext(ctx, "list governorates for registration", "error", err)
		return nil
	}
	return govs
}

// egyptCountryID resolves the country the registration form is scoped to.
func (h *UIHandler) egyptCountryID(ctx context.Context) int64 {
	countries, err := h.adminSvc.ListCountries(ctx)
	if err != nil || len(countries) == 0 {
		return 0
	}
	for _, c := range countries {
		if c.Code == "EG" {
			return c.ID
		}
	}
	return countries[0].ID
}

// cityBelongsToGovernorate reports whether a submitted city is real and, when a
// governorate was submitted too, sits inside it.
//
// Both ids come from a form. The city decides which suppliers can reach this
// pharmacy, so accepting one that names nothing — or one from a different
// governorate than the person picked — creates an account against a coverage
// area nobody chose.
func (h *UIHandler) cityBelongsToGovernorate(ctx context.Context, cityID int64, governorateID string) bool {
	if h.adminSvc == nil {
		return true
	}
	countryID := h.egyptCountryID(ctx)
	if countryID == 0 {
		return true
	}
	cities, err := h.adminSvc.ListCities(ctx, countryID)
	if err != nil {
		h.log.ErrorContext(ctx, "validate city: list cities", "error", err)
		return false
	}

	wantGov, _ := strconv.ParseInt(strings.TrimSpace(governorateID), 10, 64)
	for _, c := range cities {
		if c == nil || c.ID != cityID {
			continue
		}
		if wantGov <= 0 {
			return true
		}
		return c.GovernorateID != nil && *c.GovernorateID == wantGov
	}
	return false
}
