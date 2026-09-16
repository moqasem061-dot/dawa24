package ui_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// خط سير التوصيل at the handler boundary.
//
// The route page renders without an org service, so it has no warehouse to
// start from; that is deliberately the case under test as well as the harness
// limitation, because it is the state a courier is in before they grant
// location permission and the screen has to work there too.

// routeShipmentAt is one parcel bound for a place, for the route screen.
func routeShipmentAt(id int64, number string, lat, lon float64) *commerce.OrderShipment {
	sh := deliveryTestShipment()
	sh.ID = id
	sh.ShipmentNumber = number
	sh.CustomerBranchLatitude = &lat
	sh.CustomerBranchLongitude = &lon
	sh.CustomerOrgName = i18n.New("صيدلية "+number, "Pharmacy "+number)
	sh.TotalAmount = money.FromMinor(50000)
	return sh
}

func TestDeliveryRoutePageOrdersTheRoundAndOffersNavigation(t *testing.T) {
	round := []*commerce.OrderShipment{
		routeShipmentAt(101, "SH-A", 30.10, 31.10),
		routeShipmentAt(102, "SH-B", 30.02, 31.02),
		routeShipmentAt(103, "SH-C", 30.06, 31.06),
	}
	h := deliveryHandler(&courierMockCommerceRepo{shipment: round[0], round: round})

	rr := httptest.NewRecorder()
	// A live fix in the query string is what the browser sends after the
	// courier taps "إعادة الحساب من موقعي".
	h.VendorDeliveryRoutePage(rr, deliveryRequest(t, http.MethodGet,
		"/vendor/delivery/route?lat=30.000000&lon=31.000000", nil,
		courierActor(assignedCourierID), nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()

	for _, want := range []string{
		"خط سير التوصيل",
		"المحطة التالية",
		"ترتيب المحطات",
		"courier-route-map",
		"/vendor/delivery/route.json",
		"google.com/maps/dir",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("route page is missing %q", want)
		}
	}

	// The nearest pharmacy to the origin must be named first on the page. If
	// the far one leads, the courier drives the wrong way.
	first := strings.Index(body, "SH-B")
	far := strings.Index(body, "SH-A")
	if first < 0 || far < 0 {
		t.Fatalf("both parcels should be listed: SH-B at %d, SH-A at %d", first, far)
	}
	if first > far {
		t.Error("the far stop is listed before the near one: the round is not ordered")
	}
}

func TestDeliveryRouteJSONCarriesTheOrderedStops(t *testing.T) {
	round := []*commerce.OrderShipment{
		routeShipmentAt(101, "SH-A", 30.10, 31.10),
		routeShipmentAt(102, "SH-B", 30.02, 31.02),
	}
	h := deliveryHandler(&courierMockCommerceRepo{shipment: round[0], round: round})

	rr := httptest.NewRecorder()
	h.VendorDeliveryRouteData(rr, deliveryRequest(t, http.MethodGet,
		"/vendor/delivery/route.json?lat=30.0&lon=31.0", nil,
		courierActor(assignedCourierID), nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected JSON, got %q", ct)
	}
	// The plan is per-courier and moves with them; a cache would serve
	// yesterday's round.
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("expected no-store, got %q", cc)
	}

	var payload struct {
		HasOrigin    bool `json:"has_origin"`
		OriginIsLive bool `json:"origin_is_live"`
		Stops        []struct {
			Seq     int     `json:"seq"`
			Lat     float64 `json:"lat"`
			MapsURL string  `json:"maps_url"`
		} `json:"stops"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if !payload.HasOrigin || !payload.OriginIsLive {
		t.Error("a fix supplied in the query string must be reported as a live origin")
	}
	if len(payload.Stops) != 2 {
		t.Fatalf("expected 2 stops, got %d", len(payload.Stops))
	}
	if payload.Stops[0].Seq != 1 || payload.Stops[1].Seq != 2 {
		t.Error("stops are not numbered in visiting order")
	}
	if payload.Stops[0].Lat != 30.02 {
		t.Errorf("the near stop should be first; got lat %v", payload.Stops[0].Lat)
	}
	if !strings.Contains(payload.Stops[0].MapsURL, "30.020000,31.020000") {
		t.Errorf("stop link does not carry its coordinates: %s", payload.Stops[0].MapsURL)
	}
}

func TestDeliveryRoutePageWithNothingToCarry(t *testing.T) {
	h := deliveryHandler(&courierMockCommerceRepo{shipment: deliveryTestShipment()})

	rr := httptest.NewRecorder()
	h.VendorDeliveryRoutePage(rr, deliveryRequest(t, http.MethodGet, "/vendor/delivery/route", nil,
		courierActor(unassignedCourierID), nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "لا توجد طرود جارية بعهدتك") {
		t.Error("an empty round must say so rather than render a blank map")
	}
}

func TestDeliveryRouteJSONRefusesAnAnonymousCaller(t *testing.T) {
	h := deliveryHandler(&courierMockCommerceRepo{shipment: deliveryTestShipment()})

	rr := httptest.NewRecorder()
	h.VendorDeliveryRouteData(rr, httptest.NewRequest(http.MethodGet, "/vendor/delivery/route.json", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a request with no actor, got %d", rr.Code)
	}
}
