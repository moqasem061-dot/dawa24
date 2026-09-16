package commerce

import (
	"math"
	"sort"
	"strconv"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The delivery round, as a route rather than a list.
//
// A مندوب holding six parcels does not have six jobs; they have one journey
// with six stops, and the order of those stops is most of the day. Sorting by
// assignment time — which is what the board does, and rightly, because it is
// the fairness rule — routinely sends a courier across the city and back for
// the sake of a parcel that was handed over ten minutes earlier.
//
// This file is the geometry of that journey and nothing else: no I/O, no SQL,
// no clock. Given a starting point and the parcels on a round it answers "in
// what order, and how far". The service supplies the parcels, the handler
// supplies the starting point, and the view draws it.
//
// The optimisation is a nearest-neighbour construction improved by 2-opt. That
// is the standard treatment for an open travelling-salesman path and it is the
// right one here for three reasons: a courier's round is small (single digits
// to low tens of stops), the result must be identical for the same input every
// time it is asked for, and it must be computed inside one HTTP request with
// no external service. Exact optimality is not the goal; not doubling back is.
//
// Distance is straight-line. Roads are longer than that and the ratio is not
// constant, so the numbers here are a plan, not an ETA, and the templates say
// so. What straight-line distance is reliably good enough for is *ordering*:
// at city scale the road distance between two points is a monotonic-enough
// function of the crow-flight distance that the visiting order barely changes.
// Turn-by-turn navigation is then handed to the map application the courier
// already has, one leg at a time, from real coordinates.

// GeoPoint is a location on the Earth, in decimal degrees.
type GeoPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// IsSet reports whether the point carries a usable fix. Both zero is the
// value a missing coordinate arrives as from the database and from a browser
// that refused geolocation; it is in the Gulf of Guinea, so no Egyptian branch
// can legitimately hold it.
func (p GeoPoint) IsSet() bool {
	return (p.Lat != 0 || p.Lon != 0) &&
		p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180
}

// earthRadiusMeters is the mean radius, which is the usual choice for
// haversine over city distances.
const earthRadiusMeters = 6371000.0

// DistanceMeters is the great-circle distance between two points.
func DistanceMeters(a, b GeoPoint) float64 {
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := lat2 - lat1
	dLon := (b.Lon - a.Lon) * math.Pi / 180

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusMeters * math.Asin(math.Min(1, math.Sqrt(h)))
}

// RouteStop is one place the courier stops, with everything they are carrying
// for it.
//
// Two parcels for the same pharmacy branch are one stop. That is not a
// cosmetic grouping: routing them as two stops lets the optimiser interleave
// another destination between them, which is exactly the wasted journey this
// feature exists to remove.
type RouteStop struct {
	// Seq is the stop's position on the round, from 1.
	Seq   int      `json:"seq"`
	Point GeoPoint `json:"point"`

	PharmacyName i18n.Text `json:"pharmacy_name"`
	BranchName   i18n.Text `json:"branch_name"`
	Address      string    `json:"address"`
	Phone        string    `json:"phone"`
	ManagerName  string    `json:"manager_name,omitempty"`

	Shipments []*OrderShipment `json:"-"`

	// LegMeters is the distance from the previous stop, or from the origin for
	// the first. CumulativeMeters is the distance travelled to reach here.
	LegMeters        float64 `json:"leg_meters"`
	CumulativeMeters float64 `json:"cumulative_meters"`

	// Collect is the cash owed across every parcel dropped here, and Units the
	// number of packs. Both are what the courier needs before knocking, not
	// after opening each parcel.
	Collect money.Amount `json:"collect"`
	Units   int          `json:"units"`
	// MaxWaitingHours is the age of the oldest parcel at this stop. It drives
	// the late marker; it does not reorder the route.
	MaxWaitingHours int `json:"max_waiting_hours"`
}

// IsOverdue reports whether anything at this stop has been carried past the
// display threshold.
func (s RouteStop) IsOverdue() bool { return s.MaxWaitingHours >= CourierOverdueHours }

// ShipmentIDs lists the parcels dropped at this stop, for deep links.
func (s RouteStop) ShipmentIDs() []int64 {
	out := make([]int64, 0, len(s.Shipments))
	for _, sh := range s.Shipments {
		if sh != nil {
			out = append(out, sh.ID)
		}
	}
	return out
}

// CourierRoute is one courier's planned round.
type CourierRoute struct {
	// Origin is where the round starts: the courier's live position when the
	// browser gave one, otherwise the warehouse they collect from.
	Origin GeoPoint `json:"origin"`
	// OriginIsLive distinguishes those two, because the view must not claim a
	// courier is at the warehouse when it is only assuming so.
	OriginIsLive bool `json:"origin_is_live"`
	HasOrigin    bool `json:"has_origin"`

	Stops []RouteStop `json:"stops"`

	// Unlocatable are the parcels no stop could be built for, because the
	// receiving branch has no coordinates. They are not dropped: a parcel
	// missing from a courier's screen is a parcel that does not get delivered.
	Unlocatable []*OrderShipment `json:"-"`

	TotalMeters   float64      `json:"total_meters"`
	TotalCollect  money.Amount `json:"total_collect"`
	ShipmentCount int          `json:"shipment_count"`
	OverdueCount  int          `json:"overdue_count"`
}

// StopCount is the number of places to visit.
func (r *CourierRoute) StopCount() int {
	if r == nil {
		return 0
	}
	return len(r.Stops)
}

// IsEmpty reports whether there is nothing to draw.
func (r *CourierRoute) IsEmpty() bool {
	return r == nil || (len(r.Stops) == 0 && len(r.Unlocatable) == 0)
}

// NextStop is the one the courier should drive to now.
func (r *CourierRoute) NextStop() *RouteStop {
	if r == nil || len(r.Stops) == 0 {
		return nil
	}
	return &r.Stops[0]
}

// TotalKm is the planned distance, rounded to one decimal for display.
func (r *CourierRoute) TotalKm() float64 {
	if r == nil {
		return 0
	}
	return math.Round(r.TotalMeters/100) / 10
}

// BuildCourierRoute groups parcels into stops and orders them.
//
// origin may be unset. With no starting point there is nothing to be nearest
// to, so the round is ordered by how long each stop's oldest parcel has been
// carried — the same fairness rule the board uses — and the view says the
// route is unordered rather than pretending to a plan it does not have.
func BuildCourierRoute(origin GeoPoint, shipments []*OrderShipment) *CourierRoute {
	route := &CourierRoute{Origin: origin, HasOrigin: origin.IsSet()}

	byPlace := map[string]*RouteStop{}
	var order []string
	for _, sh := range shipments {
		if sh == nil || sh.IsClosed() {
			continue
		}
		if !sh.HasExactCoordinates() {
			route.Unlocatable = append(route.Unlocatable, sh)
			continue
		}
		key := placeKey(*sh.CustomerBranchLatitude, *sh.CustomerBranchLongitude)
		stop, seen := byPlace[key]
		if !seen {
			stop = &RouteStop{
				Point:        GeoPoint{Lat: *sh.CustomerBranchLatitude, Lon: *sh.CustomerBranchLongitude},
				PharmacyName: sh.CustomerOrgName,
				BranchName:   sh.CustomerBranchName,
				Address:      sh.CustomerBranchAddress,
				Phone:        sh.CustomerBranchPhone,
				ManagerName:  sh.CustomerManagerName,
				Collect:      money.Zero,
			}
			byPlace[key] = stop
			order = append(order, key)
		}
		stop.Shipments = append(stop.Shipments, sh)
		stop.Units += sh.TotalUnitsCount()
		if sum, err := stop.Collect.Add(sh.CourierCollection().Amount); err == nil {
			stop.Collect = sum
		}
		if h := sh.WaitingHours(); h > stop.MaxWaitingHours {
			stop.MaxWaitingHours = h
		}
	}

	stops := make([]RouteStop, 0, len(order))
	for _, key := range order {
		stops = append(stops, *byPlace[key])
	}
	if len(stops) == 0 {
		route.TotalCollect = money.Zero
		route.ShipmentCount = len(route.Unlocatable)
		return route
	}

	if route.HasOrigin {
		stops = optimiseOpenPath(origin, stops)
	} else {
		sort.SliceStable(stops, func(i, j int) bool {
			return stops[i].MaxWaitingHours > stops[j].MaxWaitingHours
		})
	}

	route.Stops = stops
	measure(route)
	return route
}

// placeKey folds coordinates onto roughly an eleven-metre grid, which is
// finer than any two pharmacies that are actually different doors and coarser
// than the jitter between two records of the same door.
func placeKey(lat, lon float64) string {
	return strconv.FormatFloat(math.Round(lat*1e4)/1e4, 'f', 4, 64) + "," +
		strconv.FormatFloat(math.Round(lon*1e4)/1e4, 'f', 4, 64)
}

// measure fills in the per-leg and whole-route numbers once the order is
// settled, and stamps the sequence the view prints on each pin.
func measure(r *CourierRoute) {
	cursor := r.Origin
	cumulative := 0.0
	total := money.Zero
	parcels := len(r.Unlocatable)

	for i := range r.Stops {
		s := &r.Stops[i]
		s.Seq = i + 1
		if r.HasOrigin || i > 0 {
			s.LegMeters = DistanceMeters(cursor, s.Point)
			cumulative += s.LegMeters
		}
		s.CumulativeMeters = cumulative
		cursor = s.Point

		parcels += len(s.Shipments)
		if sum, err := total.Add(s.Collect); err == nil {
			total = sum
		}
		if s.IsOverdue() {
			r.OverdueCount++
		}
	}
	for _, sh := range r.Unlocatable {
		if sum, err := total.Add(sh.CourierCollection().Amount); err == nil {
			total = sum
		}
	}

	r.TotalMeters = cumulative
	r.TotalCollect = total
	r.ShipmentCount = parcels
}

// optimiseOpenPath orders the stops into a short journey from origin.
//
// It is an open path: the courier does not drive back to where they started,
// so the last leg costs nothing and the last stop should be the far one. That
// is the difference between this and a textbook TSP tour, and getting it wrong
// is what makes a naive implementation end the day back across the city.
func optimiseOpenPath(origin GeoPoint, stops []RouteStop) []RouteStop {
	if len(stops) < 2 {
		return stops
	}
	return twoOpt(origin, nearestNeighbour(origin, stops))
}

// nearestNeighbour builds the first draft: always drive to the closest place
// not yet visited. It is a good route roughly a quarter of the time and a
// reasonable one nearly always, which is what 2-opt needs to start from.
func nearestNeighbour(origin GeoPoint, stops []RouteStop) []RouteStop {
	remaining := append([]RouteStop(nil), stops...)
	out := make([]RouteStop, 0, len(remaining))
	cursor := origin

	for len(remaining) > 0 {
		best, bestDist := 0, math.MaxFloat64
		for i := range remaining {
			// Ties are broken by the longest-carried parcel so the result does
			// not depend on map iteration order — two identical rounds must
			// plan identically.
			d := DistanceMeters(cursor, remaining[i].Point)
			if d < bestDist ||
				(d == bestDist && remaining[i].MaxWaitingHours > remaining[best].MaxWaitingHours) {
				best, bestDist = i, d
			}
		}
		out = append(out, remaining[best])
		cursor = remaining[best].Point
		remaining = append(remaining[:best], remaining[best+1:]...)
	}
	return out
}

// twoOptRounds caps the improvement pass. A courier's round is small enough
// that this never binds in practice; it is here so that a dispatcher's
// hundred-parcel board cannot turn one page render into a minute of CPU.
const twoOptRounds = 60

// twoOpt repeatedly reverses a segment of the path whenever doing so shortens
// it. It is what removes the crossing — the "go there, come back past where
// you were" — that nearest-neighbour leaves behind.
func twoOpt(origin GeoPoint, path []RouteStop) []RouteStop {
	n := len(path)
	if n < 3 {
		return path
	}
	pointAt := func(i int) GeoPoint {
		if i < 0 {
			return origin
		}
		return path[i].Point
	}

	for round := 0; round < twoOptRounds; round++ {
		improved := false
		for i := 0; i < n-1; i++ {
			for k := i + 1; k < n; k++ {
				// Reversing path[i..k] replaces the edge into i and the edge
				// out of k. When k is the last stop there is no edge out of
				// it, which is precisely the open-path case.
				before := DistanceMeters(pointAt(i-1), pointAt(i))
				after := DistanceMeters(pointAt(i-1), pointAt(k))
				if k+1 < n {
					before += DistanceMeters(pointAt(k), pointAt(k+1))
					after += DistanceMeters(pointAt(i), pointAt(k+1))
				}
				if after < before-0.5 {
					reverse(path[i : k+1])
					improved = true
				}
			}
		}
		if !improved {
			break
		}
	}
	return path
}

func reverse(s []RouteStop) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
