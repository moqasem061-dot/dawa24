package pages

import (
	"encoding/json"
	"strconv"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/components"
)

// Egypt's geography, as combobox options.
//
// Twenty-seven governorates and three hundred and fifty-one cities is small
// enough to ship with the page — about 12 KB — so the pickers filter locally
// and work with no network round trip at all. A search endpoint would be the
// right shape for a list that could not be shipped; this one can.
//
// The city list carries its governorate in Parent, which is what lets the
// second combobox follow the first: selecting a governorate filters the cities
// and clears any city that no longer belongs.

// GovernorateOptions turns the governorate list into combobox options.
func GovernorateOptions(govs []*platformadmin.Governorate, lang string) []components.ComboboxOption {
	out := make([]components.ComboboxOption, 0, len(govs))
	for _, g := range govs {
		if g == nil {
			continue
		}
		out = append(out, components.ComboboxOption{
			ID:    strconv.FormatInt(g.ID, 10),
			Label: geoName(g.Name, lang),
			// The other language is searchable but not shown: someone typing
			// "Cairo" should find القاهرة.
			Hint: geoAltName(g.Name, lang),
		})
	}
	return out
}

// CityOptions turns the city list into combobox options, each tagged with the
// governorate it belongs to, including governorate badge and searchable hint.
func CityOptions(cities []*platformadmin.City, lang string) []components.ComboboxOption {
	out := make([]components.ComboboxOption, 0, len(cities))
	for _, c := range cities {
		if c == nil {
			continue
		}
		parent := ""
		if c.GovernorateID != nil && *c.GovernorateID > 0 {
			parent = strconv.FormatInt(*c.GovernorateID, 10)
		}
		badge := ""
		hint := geoAltName(c.Name, lang)
		if c.GovernorateName != nil {
			govName := geoName(*c.GovernorateName, lang)
			badge = govName
			govAlt := geoAltName(*c.GovernorateName, lang)
			if hint != "" {
				hint = hint + " - " + govName
			} else {
				hint = govName
			}
			if govAlt != "" {
				hint = hint + " " + govAlt
			}
		}
		out = append(out, components.ComboboxOption{
			ID:     strconv.FormatInt(c.ID, 10),
			Label:  geoName(c.Name, lang),
			Hint:   hint,
			Badge:  badge,
			Parent: parent,
		})
	}
	return out
}

// CitiesCoordinatesJSON returns a JSON map of city ID to [lat, lon, govID, radius].
// It is embedded in pages so selecting a city can immediately pan the map and adjust coverage radius.
func CitiesCoordinatesJSON(cities []*platformadmin.City) string {
	coords := make(map[string][]any, len(cities))
	for _, c := range cities {
		if c == nil {
			continue
		}
		nameAr, nameEn := "", ""
		if c.Name != nil {
			nameAr = c.Name.Get("ar")
			nameEn = c.Name.Get("en")
		}
		govID := int64(0)
		if c.GovernorateID != nil {
			govID = *c.GovernorateID
		}
		coords[strconv.FormatInt(c.ID, 10)] = []any{c.Latitude, c.Longitude, govID, c.NormalizedRadius(), nameAr, nameEn, c.IsCapital}
	}
	b, err := json.Marshal(coords)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// GovernoratesCoordinatesJSON returns a JSON map of governorate ID to [lat, lon, radius] coordinates.
// It is embedded in pages so selecting a governorate can immediately pan the map.
func GovernoratesCoordinatesJSON(govs []*platformadmin.Governorate) string {
	coords := make(map[string][]any, len(govs))
	for _, g := range govs {
		if g == nil {
			continue
		}
		if g.Latitude != 0 || g.Longitude != 0 {
			coords[strconv.FormatInt(g.ID, 10)] = []any{g.Latitude, g.Longitude, g.NormalizedRadius()}
		}
	}
	b, err := json.Marshal(coords)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// SelectedGeoLabel finds the display name for a previously chosen id, so a
// failed submit re-renders the picker showing what the person had picked rather
// than an empty box.
func SelectedGeoLabel(options []components.ComboboxOption, id string) string {
	if id == "" {
		return ""
	}
	for _, o := range options {
		if o.ID == id {
			return o.Label
		}
	}
	return ""
}

func geoName(name i18n.Text, lang string) string {
	if v := name.Get(i18n.ParseLang(lang)); v != "" {
		return v
	}
	if v := name.Get(i18n.AR); v != "" {
		return v
	}
	return name.Get(i18n.EN)
}

func geoAltName(name i18n.Text, lang string) string {
	primary := geoName(name, lang)
	for _, l := range []i18n.Lang{i18n.AR, i18n.EN} {
		if v := name.Get(l); v != "" && v != primary {
			return v
		}
	}
	return ""
}
