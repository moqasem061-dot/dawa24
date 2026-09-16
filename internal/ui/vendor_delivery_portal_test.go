package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// إدارة الشحنات, end to end at the handler boundary.
//
// The behaviour these tests hold is the one the feature exists to create: a
// parcel is shown to, and closable by, the delivery representative it was
// assigned to — and to nobody else. The portal it replaced had no notion of
// "assigned to", so none of this could be asserted about it.

const (
	deliveryOrgID       int64 = 77
	assignedCourierID   int64 = 4200
	unassignedCourierID int64 = 4201
	dispatcherUserID    int64 = 4300
)

func deliveryTestShipment() *commerce.OrderShipment {
	assigned := time.Now().Add(-30 * time.Hour)
	courier := assignedCourierID
	return &commerce.OrderShipment{
		ID:                      101,
		OrderID:                 505,
		OrganizationID:          deliveryOrgID,
		ShipmentNumber:          "SH-2026-001",
		TrackingNumber:          "TRK-987654",
		Status:                  commerce.StatusShipped,
		DeliveryCode:            "654321",
		PaymentMethod:           "cod",
		TotalAmount:             money.FromMinor(125000),
		CourierUserID:           &courier,
		CourierAssignedAt:       &assigned,
		CourierName:             "محمود المندوب",
		CustomerOrgName:         i18n.New("صيدلية النور الحديثة", "Al-Noor Modern Pharmacy"),
		CustomerBranchName:      i18n.New("فرع المعادي", "Maadi Branch"),
		CustomerBranchAddress:   "شارع النصر، أمام مستشفى المعادي، القاهرة",
		CustomerBranchPhone:     "01012345678",
		CustomerBranchLatitude:  func(f float64) *float64 { return &f }(29.9602),
		CustomerBranchLongitude: func(f float64) *float64 { return &f }(31.2825),
		Lines: []*commerce.OrderLine{
			{ID: 1, ProductName: i18n.New("بنادول إكسترا 24 قرص", "Panadol Extra"), Quantity: 10,
				UnitPrice: money.FromMinor(4500), TotalPrice: money.FromMinor(45000)},
			{ID: 2, ProductName: i18n.New("أوجمنتين 1 جم 14 قرص", "Augmentin 1g"), Quantity: 5,
				UnitPrice: money.FromMinor(8500), TotalPrice: money.FromMinor(42500)},
			{ID: 3, ProductName: i18n.New("كونجستال 20 قرص", "Congestal"), Quantity: 8,
				UnitPrice: money.FromMinor(2500), TotalPrice: money.FromMinor(20000)},
			{ID: 4, ProductName: i18n.New("كاتافلام 50 مجم", "Cataflam 50mg"), Quantity: 12,
				UnitPrice: money.FromMinor(3300), TotalPrice: money.FromMinor(39600)},
			{ID: 5, ProductName: i18n.New("أوميبرازول 20 مجم", "Omeprazole 20mg"), Quantity: 6,
				UnitPrice: money.FromMinor(4000), TotalPrice: money.FromMinor(24000)},
			{ID: 6, ProductName: i18n.New("فيتامين سي 1000 مجم", "Vitamin C 1000mg"), Quantity: 15,
				UnitPrice: money.FromMinor(2000), TotalPrice: money.FromMinor(30000)},
		},
	}
}

func deliveryHandler(repo commerce.Repository) *ui.UIHandler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return ui.NewUIHandler(nil, nil, nil, commerce.NewService(repo, log),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log)
}

// courierActor is a مندوب: they hold the portal and the actions that close a
// parcel, and nothing that lets them decide whose round it is on.
func courierActor(userID int64) authctx.Actor {
	a := authctx.Actor{
		UserID:         userID,
		OrganizationID: deliveryOrgID,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Scope:          rbac.ScopeVendor,
	}
	a.Grants([]string{"vendor.delivery.view", "vendor.delivery.update"})
	return a
}

// dispatcherActor plans rounds. They see every parcel and may assign it, and
// deliberately may not sign for one.
func dispatcherActor() authctx.Actor {
	a := authctx.Actor{
		UserID:         dispatcherUserID,
		OrganizationID: deliveryOrgID,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Scope:          rbac.ScopeVendor,
	}
	a.Grants([]string{"vendor.delivery.view", "vendor.delivery.assign", "vendor.order.view"})
	return a
}

func deliveryRequest(t *testing.T, method, path string, form url.Values, actor authctx.Actor, params map[string]string) *http.Request {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	ctx := authctx.WithActor(req.Context(), actor)
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	}
	return req.WithContext(ctx)
}

// TestDeliveryPortalShowsTheCourierTheirOwnRound is the whole point of the
// feature: the parcels a مندوب sees are the ones assigned to them.
func TestDeliveryPortalShowsTheCourierTheirOwnRound(t *testing.T) {
	h := deliveryHandler(&courierMockCommerceRepo{shipment: deliveryTestShipment()})

	rr := httptest.NewRecorder()
	h.VendorDeliveryPortalPage(rr, deliveryRequest(t, http.MethodGet, "/vendor/delivery", nil,
		courierActor(assignedCourierID), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("portal returned %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"SH-2026-001", "صيدلية النور الحديثة", "إدارة الشحنات", "تاريخ الإسناد"} {
		if !strings.Contains(body, want) {
			t.Errorf("the board is missing %q", want)
		}
	}
	// Assigned 30 hours ago: the board has to say how long, because that is
	// the order it is sorted in and the reason one parcel comes before another.
	if !strings.Contains(body, "بعهدتك منذ 1 يوم") {
		t.Error("the board does not show how long the parcel has been waiting")
	}

	// A different courier's board is empty rather than somebody else's round.
	rr = httptest.NewRecorder()
	h.VendorDeliveryPortalPage(rr, deliveryRequest(t, http.MethodGet, "/vendor/delivery", nil,
		courierActor(unassignedCourierID), nil))
	if strings.Contains(rr.Body.String(), "SH-2026-001") {
		t.Error("a courier was shown a parcel assigned to somebody else")
	}
}

// TestDeliveryPortalRefusesADispatchTabToACourier. The tab is a request, not
// an authority: asking for the company-wide pool without the assign grant
// falls back to the caller's own round rather than answering it.
func TestDeliveryPortalRefusesADispatchTabToACourier(t *testing.T) {
	sh := deliveryTestShipment()
	sh.CourierUserID = nil
	sh.CourierAssignedAt = nil
	h := deliveryHandler(&courierMockCommerceRepo{shipment: sh})

	rr := httptest.NewRecorder()
	h.VendorDeliveryPortalPage(rr, deliveryRequest(t, http.MethodGet,
		"/vendor/delivery?queue=unassigned", nil, courierActor(assignedCourierID), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("portal returned %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "SH-2026-001") {
		t.Error("a courier reached the unassigned pool by asking for it in the query string")
	}
	if strings.Contains(body, "بانتظار الإسناد (") {
		t.Error("the dispatch tabs are offered to a courier who cannot dispatch")
	}
}

// TestDeliveryShipmentPageIsScopedToTheAssignedCourier.
func TestDeliveryShipmentPageIsScopedToTheAssignedCourier(t *testing.T) {
	sh := deliveryTestShipment()
	sh.Status = commerce.StatusOutForDelivery
	h := deliveryHandler(&courierMockCommerceRepo{shipment: sh})
	params := map[string]string{"id": "101"}

	rr := httptest.NewRecorder()
	h.VendorDeliveryShipmentPage(rr, deliveryRequest(t, http.MethodGet, "/vendor/delivery/101", nil,
		courierActor(assignedCourierID), params))
	if rr.Code != http.StatusOK {
		t.Fatalf("the assigned courier got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"SH-2026-001",
		"شارع النصر", // the address
		"تأكيد تسليم الطرد بالكود",    // the handover form
		"المبلغ المطلوب تحصيله نقداً", // cash on delivery
		"1,250.00",           // the amount itself
		"courier-branch-map", // the GPS pin
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the parcel screen is missing %q", want)
		}
	}

	// Another courier is sent back to the board rather than shown the
	// pharmacy's address and the cash due.
	rr = httptest.NewRecorder()
	h.VendorDeliveryShipmentPage(rr, deliveryRequest(t, http.MethodGet, "/vendor/delivery/101", nil,
		courierActor(unassignedCourierID), params))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("an unassigned courier got %d, want a redirect away", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.HasPrefix(loc, "/vendor/delivery?") {
		t.Errorf("an unassigned courier was sent to %q", loc)
	}
}

// TestDeliveryHandoverRequiresTheRightCode.
func TestDeliveryHandoverRequiresTheRightCode(t *testing.T) {
	sh := deliveryTestShipment()
	h := deliveryHandler(&courierMockCommerceRepo{shipment: sh})
	params := map[string]string{"id": "101"}

	rr := httptest.NewRecorder()
	h.VendorDeliveryVerifySubmit(rr, deliveryRequest(t, http.MethodPost, "/vendor/delivery/101/verify",
		url.Values{"delivery_code": {"000000"}}, courierActor(assignedCourierID), params))
	if sh.Status == commerce.StatusDelivered {
		t.Fatal("a wrong code closed the parcel")
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "notice=error") {
		t.Errorf("a wrong code did not report an error; redirected to %q", loc)
	}

	rr = httptest.NewRecorder()
	h.VendorDeliveryVerifySubmit(rr, deliveryRequest(t, http.MethodPost, "/vendor/delivery/101/verify",
		url.Values{"delivery_code": {"654321"}, "notes": {"تم التسليم للصيدلي المسؤول"}},
		courierActor(assignedCourierID), params))
	if sh.Status != commerce.StatusDelivered {
		t.Fatalf("the right code did not close the parcel; status is %s", sh.Status)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "notice=success") {
		t.Errorf("a successful handover did not report success; redirected to %q", loc)
	}
}

// TestDeliveryHandoverRefusesAParcelOnAnotherRound. Holding the waybill number
// is what used to be sufficient; being the assigned representative is what is
// sufficient now.
func TestDeliveryHandoverRefusesAParcelOnAnotherRound(t *testing.T) {
	sh := deliveryTestShipment()
	h := deliveryHandler(&courierMockCommerceRepo{shipment: sh})

	rr := httptest.NewRecorder()
	h.VendorDeliveryVerifySubmit(rr, deliveryRequest(t, http.MethodPost, "/vendor/delivery/101/verify",
		url.Values{"delivery_code": {"654321"}}, courierActor(unassignedCourierID),
		map[string]string{"id": "101"}))
	if sh.Status == commerce.StatusDelivered {
		t.Fatal("a courier closed a parcel that was not assigned to them")
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "notice=error") {
		t.Errorf("the refusal was not reported; redirected to %q", loc)
	}
}

// TestCourierCannotMarkDeliveredWithoutTheCode. The status form moves a parcel
// along the round; it must not become a way around the pharmacy's confirmation.
func TestCourierCannotMarkDeliveredWithoutTheCode(t *testing.T) {
	sh := deliveryTestShipment()
	h := deliveryHandler(&courierMockCommerceRepo{shipment: sh})

	rr := httptest.NewRecorder()
	h.VendorDeliveryStatusSubmit(rr, deliveryRequest(t, http.MethodPost, "/vendor/delivery/101/status",
		url.Values{"status": {string(commerce.StatusDelivered)}}, courierActor(assignedCourierID),
		map[string]string{"id": "101"}))
	if sh.Status == commerce.StatusDelivered {
		t.Fatal("a parcel was marked delivered without the pharmacy's code")
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "notice=error") {
		t.Errorf("the refusal was not reported; redirected to %q", loc)
	}
}

// TestCourierAdvancesTheirOwnParcel covers the transitions that are the
// courier's own claim about themselves.
func TestCourierAdvancesTheirOwnParcel(t *testing.T) {
	sh := deliveryTestShipment()
	h := deliveryHandler(&courierMockCommerceRepo{shipment: sh})

	rr := httptest.NewRecorder()
	h.VendorDeliveryStatusSubmit(rr, deliveryRequest(t, http.MethodPost, "/vendor/delivery/101/status",
		url.Values{"status": {string(commerce.StatusOutForDelivery)}}, courierActor(assignedCourierID),
		map[string]string{"id": "101"}))
	if sh.Status != commerce.StatusOutForDelivery {
		t.Fatalf("status is %s, want out_for_delivery", sh.Status)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "notice=success") {
		t.Errorf("redirected to %q", loc)
	}
}

// TestDispatcherSeesTheWholeBoardButDoesNotSign.
func TestDispatcherSeesTheWholeBoardButDoesNotSign(t *testing.T) {
	h := deliveryHandler(&courierMockCommerceRepo{shipment: deliveryTestShipment()})

	rr := httptest.NewRecorder()
	h.VendorDeliveryShipmentPage(rr, deliveryRequest(t, http.MethodGet, "/vendor/delivery/101", nil,
		dispatcherActor(), map[string]string{"id": "101"}))
	if rr.Code != http.StatusOK {
		t.Fatalf("a dispatcher got %d opening a parcel on somebody else's round, want 200", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "تأكيد تسليم الطرد بالكود") {
		t.Error("the handover form is offered to a dispatcher who is not carrying the parcel")
	}
	if !strings.Contains(body, "هذا الطرد غير مسند إليك") {
		t.Error("the dispatcher is not told why the delivery actions are absent")
	}
}

// TestCourierMarkDeliveryFailedShowsFailedClosedCard verifies that marking delivery
// as failed flags the shipment as failed and renders the failure closed card,
// never the success handover or success message.
func TestCourierMarkDeliveryFailedShowsFailedClosedCard(t *testing.T) {
	sh := deliveryTestShipment()
	h := deliveryHandler(&courierMockCommerceRepo{shipment: sh})

	params := map[string]string{"id": "101"}
	rr := httptest.NewRecorder()
	h.VendorDeliveryStatusSubmit(rr, deliveryRequest(t, http.MethodPost, "/vendor/delivery/101/status",
		url.Values{"status": {string(commerce.StatusFailed)}, "notes": {"الصيدلية مغلقة وتم الاتصال بدون رد"}},
		courierActor(assignedCourierID), params))

	if sh.Status != commerce.StatusFailed {
		t.Fatalf("status is %s, want failed", sh.Status)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "notice=success") {
		t.Errorf("redirected to %q", loc)
	}

	// Now render the shipment detail page for the courier
	rr = httptest.NewRecorder()
	h.VendorDeliveryShipmentPage(rr, deliveryRequest(t, http.MethodGet, "/vendor/delivery/101", nil,
		courierActor(assignedCourierID), params))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "تعذّر تسليم الطرد للصيدلية") {
		t.Error("expected failed closed card with 'تعذّر تسليم الطرد للصيدلية', not found")
	}
	if !strings.Contains(body, "الصيدلية مغلقة وتم الاتصال بدون رد") {
		t.Error("expected failure notes to be rendered on closed card")
	}
	if strings.Contains(body, "تم تسليم الطرد وتوثيقه بنجاح") {
		t.Error("failure page incorrectly contains 'تم تسليم الطرد وتوثيقه بنجاح'")
	}
	if strings.Contains(body, "تأكيد تسليم الطرد بالكود") {
		t.Error("failure page incorrectly contains handover PIN form")
	}
}

// TestCourierBoardSeparatesCompletedAndFailedQueues verifies that the courier board
// separates delivered parcels from failed parcels and displays accurate labels and badges.
func TestCourierBoardSeparatesCompletedAndFailedQueues(t *testing.T) {
	sh := deliveryTestShipment()
	sh.Status = commerce.StatusFailed
	h := deliveryHandler(&courierMockCommerceRepo{shipment: sh})

	rr := httptest.NewRecorder()
	h.VendorDeliveryPortalPage(rr, deliveryRequest(t, http.MethodGet, "/vendor/delivery?queue=failed", nil,
		courierActor(assignedCourierID), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("portal returned %d, want 200", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "تعذر تسليمها") {
		t.Error("expected queue tab 'تعذر تسليمها' in courier delivery page")
	}
	if !strings.Contains(body, "SH-2026-001") {
		t.Error("failed queue should show the failed shipment")
	}
	if !strings.Contains(body, "تعذر تسليمه") {
		t.Error("failed shipment badge should say 'تعذر تسليمه'")
	}
	if strings.Contains(body, "بعهدتك منذ 1 يوم") {
		t.Error("failed shipment should not say 'بعهدتك منذ 1 يوم'")
	}
}
