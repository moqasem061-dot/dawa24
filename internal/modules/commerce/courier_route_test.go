package commerce

import (
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The route is the one piece of this portal a courier cannot check by eye. If
// it puts the far stop second they will drive it, so these tests assert the
// two properties that make the plan trustworthy: it visits everything exactly
// once, and it does not double back.

func shipmentAt(id int64, lat, lon float64, assignedHoursAgo int) *OrderShipment {
	assigned := time.Now().Add(-time.Duration(assignedHoursAgo) * time.Hour)
	return &OrderShipment{
		ID:                      id,
		ShipmentNumber:          "SH-" + string(rune('0'+id%10)),
		Status:                  StatusInTransit,
		PaymentMethod:           "cod",
		TotalAmount:             money.FromMajor(100),
		CustomerOrgName:         i18n.Text{"ar": "صيدلية", "en": "Pharmacy"},
		CustomerBranchName:      i18n.Text{"ar": "فرع", "en": "Branch"},
		CustomerBranchLatitude:  &lat,
		CustomerBranchLongitude: &lon,
		CourierAssignedAt:       &assigned,
	}
}

func TestDistanceMetersIsRoughlyRight(t *testing.T) {
	// Cairo Tahrir to Giza pyramids: about 13 km on the map.
	d := DistanceMeters(GeoPoint{30.0444, 31.2357}, GeoPoint{29.9792, 31.1342})
	if d < 12000 || d > 15000 {
		t.Fatalf("expected ~13 km between Tahrir and Giza, got %.0f m", d)
	}
	if DistanceMeters(GeoPoint{30, 31}, GeoPoint{30, 31}) != 0 {
		t.Fatal("a point is not zero metres from itself")
	}
}

func TestGeoPointIsSetRejectsTheNullIsland(t *testing.T) {
	cases := []struct {
		name string
		p    GeoPoint
		want bool
	}{
		{"cairo", GeoPoint{30.0444, 31.2357}, true},
		{"both zero", GeoPoint{}, false},
		{"out of range latitude", GeoPoint{95, 31}, false},
		{"out of range longitude", GeoPoint{30, 200}, false},
	}
	for _, c := range cases {
		if got := c.p.IsSet(); got != c.want {
			t.Errorf("%s: IsSet() = %v, want %v", c.name, got, c.want)
		}
	}
}

// A round laid out along one road must be driven along that road, not
// alternately up and down it. This is the failure the feature exists to stop:
// assignment order here is deliberately the worst possible driving order.
func TestBuildCourierRouteDoesNotDoubleBack(t *testing.T) {
	origin := GeoPoint{Lat: 30.00, Lon: 31.00}
	// Five stops due east of the origin, handed over in zig-zag order.
	shipments := []*OrderShipment{
		shipmentAt(1, 30.00, 31.08, 5),
		shipmentAt(2, 30.00, 31.02, 4),
		shipmentAt(3, 30.00, 31.10, 3),
		shipmentAt(4, 30.00, 31.04, 2),
		shipmentAt(5, 30.00, 31.06, 1),
	}

	route := BuildCourierRoute(origin, shipments)
	if route.StopCount() != 5 {
		t.Fatalf("expected 5 stops, got %d", route.StopCount())
	}

	// Every stop visited once, in increasing longitude — the only order that
	// crosses the line without retracing it.
	for i := 1; i < len(route.Stops); i++ {
		if route.Stops[i].Point.Lon <= route.Stops[i-1].Point.Lon {
			t.Fatalf("stop %d (lon %.2f) comes after %d (lon %.2f): the route doubles back",
				i+1, route.Stops[i].Point.Lon, i, route.Stops[i-1].Point.Lon)
		}
	}
	for i, s := range route.Stops {
		if s.Seq != i+1 {
			t.Errorf("stop at index %d carries sequence %d", i, s.Seq)
		}
	}

	// The optimal open path along the line is the span from the origin to the
	// far end, and nothing shorter is reachable.
	wantMeters := DistanceMeters(origin, GeoPoint{30.00, 31.10})
	if route.TotalMeters > wantMeters*1.01 {
		t.Fatalf("route is %.0f m, optimal is %.0f m", route.TotalMeters, wantMeters)
	}
}

func TestBuildCourierRouteGroupsOneBranchIntoOneStop(t *testing.T) {
	origin := GeoPoint{Lat: 30.00, Lon: 31.00}
	route := BuildCourierRoute(origin, []*OrderShipment{
		shipmentAt(1, 30.05, 31.05, 2),
		shipmentAt(2, 30.05, 31.05, 30),
		shipmentAt(3, 30.09, 31.09, 1),
	})

	if route.StopCount() != 2 {
		t.Fatalf("two parcels for one branch must be one stop; got %d stops", route.StopCount())
	}
	var shared *RouteStop
	for i := range route.Stops {
		if len(route.Stops[i].Shipments) == 2 {
			shared = &route.Stops[i]
		}
	}
	if shared == nil {
		t.Fatal("no stop carries both parcels for the shared branch")
	}
	if shared.Units != 0 || shared.Collect.Minor() != money.FromMajor(200).Minor() {
		t.Errorf("stop totals not summed: collect=%s", shared.Collect)
	}
	if !shared.IsOverdue() {
		t.Error("a stop holding a 30-hour-old parcel must read as late")
	}
	if route.ShipmentCount != 3 {
		t.Errorf("expected 3 parcels across the round, got %d", route.ShipmentCount)
	}
	if route.OverdueCount != 1 {
		t.Errorf("expected 1 late stop, got %d", route.OverdueCount)
	}
}

func TestBuildCourierRouteKeepsParcelsWithNoCoordinates(t *testing.T) {
	blind := shipmentAt(9, 30.05, 31.05, 1)
	blind.CustomerBranchLatitude, blind.CustomerBranchLongitude = nil, nil

	route := BuildCourierRoute(GeoPoint{30.00, 31.00}, []*OrderShipment{
		shipmentAt(1, 30.02, 31.02, 1), blind,
	})

	if len(route.Unlocatable) != 1 || route.Unlocatable[0].ID != 9 {
		t.Fatalf("a parcel with no coordinates must survive as unlocatable; got %d", len(route.Unlocatable))
	}
	if route.StopCount() != 1 {
		t.Fatalf("expected 1 mapped stop, got %d", route.StopCount())
	}
	// Its cash still counts: the courier collects it whether or not the branch
	// ever saved a pin.
	if route.TotalCollect.Minor() != money.FromMajor(200).Minor() {
		t.Errorf("unlocatable parcel excluded from the day's collection: %s", route.TotalCollect)
	}
}

func TestBuildCourierRouteWithoutOriginFallsBackToOldestFirst(t *testing.T) {
	route := BuildCourierRoute(GeoPoint{}, []*OrderShipment{
		shipmentAt(1, 30.02, 31.02, 2),
		shipmentAt(2, 30.09, 31.09, 40),
		shipmentAt(3, 30.05, 31.05, 12),
	})

	if route.HasOrigin {
		t.Fatal("an unset origin must not be reported as a starting point")
	}
	if route.StopCount() != 3 {
		t.Fatalf("expected 3 stops, got %d", route.StopCount())
	}
	if route.Stops[0].MaxWaitingHours < route.Stops[1].MaxWaitingHours ||
		route.Stops[1].MaxWaitingHours < route.Stops[2].MaxWaitingHours {
		t.Error("with no origin the round must fall back to oldest-parcel-first")
	}
	// Without a starting point there is no first leg to measure.
	if route.Stops[0].LegMeters != 0 {
		t.Errorf("first leg measured from an unknown origin: %.0f m", route.Stops[0].LegMeters)
	}
}

func TestBuildCourierRouteIsDeterministic(t *testing.T) {
	origin := GeoPoint{30.00, 31.00}
	build := func() []int64 {
		route := BuildCourierRoute(origin, []*OrderShipment{
			shipmentAt(1, 30.03, 31.07, 3),
			shipmentAt(2, 30.07, 31.03, 3),
			shipmentAt(3, 30.05, 31.05, 3),
			shipmentAt(4, 30.09, 31.01, 3),
		})
		out := make([]int64, 0, route.StopCount())
		for _, s := range route.Stops {
			out = append(out, s.Shipments[0].ID)
		}
		return out
	}
	first := build()
	for i := 0; i < 5; i++ {
		again := build()
		for j := range first {
			if first[j] != again[j] {
				t.Fatalf("plan %d differs from the first: %v vs %v", i, first, again)
			}
		}
	}
}

func TestBuildCourierRouteSkipsClosedParcels(t *testing.T) {
	closed := shipmentAt(2, 30.05, 31.05, 1)
	closed.Status = StatusDelivered

	route := BuildCourierRoute(GeoPoint{30.00, 31.00}, []*OrderShipment{
		shipmentAt(1, 30.02, 31.02, 1), closed, nil,
	})
	if route.StopCount() != 1 {
		t.Fatalf("a signed-for parcel must not appear on the round; got %d stops", route.StopCount())
	}
}

func TestEmptyRouteIsEmpty(t *testing.T) {
	route := BuildCourierRoute(GeoPoint{30, 31}, nil)
	if !route.IsEmpty() || route.NextStop() != nil || route.TotalKm() != 0 {
		t.Fatal("a round with no parcels must report as empty")
	}
}
