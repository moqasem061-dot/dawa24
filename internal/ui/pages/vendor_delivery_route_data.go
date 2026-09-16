package pages

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The route screen's data, in the two shapes it is needed in: the struct the
// template renders from, and the JSON the map re-draws itself from when the
// courier moves.
//
// Both are built from the same commerce.CourierRoute, so the pins on the map
// and the rows in the list cannot disagree about the order of the day.

// VendorDeliveryRouteData is what the page renders.
type VendorDeliveryRouteData struct {
	Route  *commerce.CourierRoute
	Counts commerce.CourierQueueCounts
	// WarehouseName labels the origin pin when the round starts at a branch
	// rather than at a live position.
	WarehouseName string
	NoticeType    string
	NoticeMsg     string
}

// RouteStopJSON is one pin and one list row, with its names already resolved
// into the reader's language — the browser has no i18n.Text.
type RouteStopJSON struct {
	Seq        int     `json:"seq"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Name       string  `json:"name"`
	Branch     string  `json:"branch"`
	Address    string  `json:"address"`
	Phone      string  `json:"phone,omitempty"`
	Parcels    int     `json:"parcels"`
	Units      int     `json:"units"`
	Collect    string  `json:"collect"`
	LegKm      float64 `json:"leg_km"`
	TotalKm    float64 `json:"total_km"`
	Overdue    bool    `json:"overdue"`
	MapsURL    string  `json:"maps_url"`
	ShipmentID int64   `json:"shipment_id"`
	Shipments  []int64 `json:"shipments"`
}

// RouteJSONPayload is the whole plan as the map consumes it.
type RouteJSONPayload struct {
	HasOrigin      bool            `json:"has_origin"`
	OriginIsLive   bool            `json:"origin_is_live"`
	OriginLat      float64         `json:"origin_lat"`
	OriginLon      float64         `json:"origin_lon"`
	Stops          []RouteStopJSON `json:"stops"`
	TotalKm        float64         `json:"total_km"`
	ShipmentCount  int             `json:"shipment_count"`
	UnlocatedCount int             `json:"unlocated_count"`
	FullRouteURL   string          `json:"full_route_url"`
}

// RouteJSON projects a plan for the browser.
func RouteJSON(route *commerce.CourierRoute, lang string) RouteJSONPayload {
	out := RouteJSONPayload{Stops: []RouteStopJSON{}}
	if route == nil {
		return out
	}
	out.HasOrigin = route.HasOrigin
	out.OriginIsLive = route.OriginIsLive
	out.OriginLat, out.OriginLon = route.Origin.Lat, route.Origin.Lon
	out.TotalKm = route.TotalKm()
	out.ShipmentCount = route.ShipmentCount
	out.UnlocatedCount = len(route.Unlocatable)
	out.FullRouteURL = FullRouteMapsURL(route)

	l := i18n.ParseLang(lang)
	for i := range route.Stops {
		s := route.Stops[i]
		row := RouteStopJSON{
			Seq:       s.Seq,
			Lat:       s.Point.Lat,
			Lon:       s.Point.Lon,
			Name:      s.PharmacyName.Get(l),
			Branch:    s.BranchName.Get(l),
			Address:   s.Address,
			Phone:     s.Phone,
			Parcels:   len(s.Shipments),
			Units:     s.Units,
			Collect:   s.Collect.String(),
			LegKm:     km(s.LegMeters),
			TotalKm:   km(s.CumulativeMeters),
			Overdue:   s.IsOverdue(),
			MapsURL:   StopMapsURL(s),
			Shipments: s.ShipmentIDs(),
		}
		if len(row.Shipments) > 0 {
			row.ShipmentID = row.Shipments[0]
		}
		out.Stops = append(out.Stops, row)
	}
	return out
}

// km rounds metres to one decimal kilometre, the only precision a
// straight-line estimate can honestly carry.
func km(meters float64) float64 { return math.Round(meters/100) / 10 }

// StopMapsURL is turn-by-turn navigation to one stop, handed to whatever map
// application the courier's phone opens Google's directions link with.
func StopMapsURL(s commerce.RouteStop) string {
	return fmt.Sprintf("https://www.google.com/maps/dir/?api=1&destination=%s,%s&travelmode=driving",
		coord(s.Point.Lat), coord(s.Point.Lon))
}

// googleWaypointLimit is how many intermediate stops Google's directions URL
// accepts. Beyond it the link is silently truncated, so the button is only
// offered for a round that fits and the per-stop links carry the rest.
const googleWaypointLimit = 9

// FullRouteMapsURL is the whole planned round as one navigation link:
// origin, every intermediate stop as a waypoint in the planned order, and the
// last stop as the destination.
//
// This is where the optimisation becomes turn-by-turn driving. We compute the
// order — which is the part Google will not do for a fixed sequence — and hand
// the sequence to the application that has the road network.
func FullRouteMapsURL(route *commerce.CourierRoute) string {
	if route == nil || len(route.Stops) == 0 {
		return ""
	}
	last := route.Stops[len(route.Stops)-1]
	v := url.Values{
		"api":         {"1"},
		"destination": {coord(last.Point.Lat) + "," + coord(last.Point.Lon)},
		"travelmode":  {"driving"},
	}
	if route.HasOrigin {
		v.Set("origin", coord(route.Origin.Lat)+","+coord(route.Origin.Lon))
	}
	if mid := route.Stops[:len(route.Stops)-1]; len(mid) > 0 {
		if len(mid) > googleWaypointLimit {
			return ""
		}
		parts := make([]string, 0, len(mid))
		for _, s := range mid {
			parts = append(parts, coord(s.Point.Lat)+","+coord(s.Point.Lon))
		}
		v.Set("waypoints", strings.Join(parts, "|"))
	}
	return "https://www.google.com/maps/dir/?" + v.Encode()
}

// coord formats a degree value for a URL: six decimals is about a tenth of a
// metre, which is more than a doorway needs and less than a float's noise.
func coord(f float64) string { return strconv.FormatFloat(f, 'f', 6, 64) }
