package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// apiGuardRepo counts every write the JSON routes could make, so a refused
// request can be shown to have changed nothing.
type apiGuardRepo struct {
	happyRepo
	paymentMethod  string
	orderStatus    commerce.OrderStatus
	otherStatus    commerce.OrderStatus
	writes         int
	created        int
	shipmentCancel []int64
}

func (r *apiGuardRepo) GetOrderByID(_ context.Context, id int64) (*commerce.Order, error) {
	return &commerce.Order{
		ID: id, CustomerID: 100, Status: r.orderStatus, PaymentMethod: r.paymentMethod,
		Shipments: []*commerce.OrderShipment{
			{ID: 11, OrderID: id, OrganizationID: 10, Status: commerce.StatusPending},
			{ID: 22, OrderID: id, OrganizationID: 20, Status: r.otherStatus},
		},
	}, nil
}
func (r *apiGuardRepo) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{ID: id, OrderID: 1, OrganizationID: 10, Status: commerce.StatusPending}, nil
}
func (r *apiGuardRepo) UpdateOrderStatus(context.Context, int64, commerce.OrderStatus, commerce.OrderStatusHistory) error {
	r.writes++
	return nil
}
func (r *apiGuardRepo) UpdateShipmentStatus(context.Context, int64, commerce.OrderStatus, commerce.OrderStatus, commerce.OrderStatusHistory) error {
	r.writes++
	return nil
}
func (r *apiGuardRepo) CancelAPIOrder(context.Context, commerce.APICancelRequest) error {
	r.writes++
	return nil
}
func (r *apiGuardRepo) CancelAPIVendorShipment(_ context.Context, request commerce.APIVendorCancelRequest) error {
	r.writes++
	r.shipmentCancel = append(r.shipmentCancel, request.ShipmentID)
	return nil
}
func (r *apiGuardRepo) CreateOrder(_ context.Context, _ *commerce.Order, _ []*commerce.OrderShipment, _ []*commerce.OrderLine) error {
	r.created++
	return nil
}

func postJSON(t *testing.T, router http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// Wallet orders are refused on the order-status and shipment-status routes for
// every actor and every status, because the website alone moves the money.
func TestAPIWalletOrdersAreRefusedOnEveryJSONStatusRoute(t *testing.T) {
	vendor := authctx.Actor{UserID: 101, OrganizationID: 10, OrgType: "vendor", Permissions: []string{"vendor.order.update"}}
	staff := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin", Permissions: []string{"commerce.admin"}}
	buyer := authctx.Actor{UserID: 100, OrganizationID: 1, OrgType: "pharmacy"}
	routes := []struct {
		name, path string
		actor      authctx.Actor
	}{
		{"order_status_vendor", "/api/v1/commerce/orders/1/status", vendor},
		{"order_status_staff", "/api/v1/commerce/orders/1/status", staff},
		{"order_status_buyer", "/api/v1/commerce/orders/1/status", buyer},
		{"shipment_status_vendor", "/api/v1/commerce/shipments/11/status", vendor},
		{"shipment_status_staff", "/api/v1/commerce/shipments/11/status", staff},
	}
	for _, route := range routes {
		for _, status := range []string{"confirmed", "cancelled", "shipped", "delivered"} {
			t.Run(route.name+"_"+status, func(t *testing.T) {
				repo := &apiGuardRepo{paymentMethod: "wallet", orderStatus: commerce.StatusPending, otherStatus: commerce.StatusPending}
				response := postJSON(t, newRouterWithActor(repo, route.actor), route.path, `{"status":"`+status+`"}`)
				if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "order.wallet_on_website") || repo.writes != 0 {
					t.Fatalf("status=%d writes=%d body=%s", response.Code, repo.writes, response.Body.String())
				}
			})
		}
	}
}

// A shipment cancel on the shipment-status route follows the same temporary rule
// as an order cancel. It is checked under lock and changes only this supplier's
// shipment.
func TestAPIShipmentCancelFollowsTheTemporaryCancelRule(t *testing.T) {
	vendor := authctx.Actor{UserID: 101, OrganizationID: 10, OrgType: "vendor", Permissions: []string{"vendor.order.update"}}
	cases := []struct {
		name                     string
		orderStatus, otherStatus commerce.OrderStatus
		wantCode                 int
		wantRefusal              string
	}{
		{"confirmed_order_refused", commerce.StatusConfirmed, commerce.StatusPending, http.StatusConflict, "order.cannot_cancel"},
		{"sibling_shipped_refused", commerce.StatusPending, commerce.StatusShipped, http.StatusConflict, "order.shipment_in_progress"},
		{"pending_order_allowed", commerce.StatusPending, commerce.StatusPending, http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &apiGuardRepo{paymentMethod: "cod", orderStatus: tc.orderStatus, otherStatus: tc.otherStatus}
			response := postJSON(t, newRouterWithActor(repo, vendor), "/api/v1/commerce/shipments/11/status", `{"status":"cancelled"}`)
			if response.Code != tc.wantCode {
				t.Fatalf("status=%d want %d body=%s", response.Code, tc.wantCode, response.Body.String())
			}
			if tc.wantRefusal != "" && !strings.Contains(response.Body.String(), tc.wantRefusal) {
				t.Fatalf("refusal %q missing from %s", tc.wantRefusal, response.Body.String())
			}
			if tc.wantCode == http.StatusOK && (len(repo.shipmentCancel) != 1 || repo.shipmentCancel[0] != 11) {
				t.Fatalf("shipment cancels=%v, want only shipment 11", repo.shipmentCancel)
			}
			if tc.wantCode != http.StatusOK && repo.writes != 0 {
				t.Fatalf("refused cancel still wrote %d rows", repo.writes)
			}
		})
	}
}

// Over the wire, the REST checkout refuses a wallet payment and a standalone
// bundle without creating an order.
func TestAPICheckoutRefusesWalletAndBundleOverHTTP(t *testing.T) {
	cases := []struct {
		name, body, code string
	}{
		{"wallet", `{"branch_id":3,"payment_method":"wallet","items":[{"vendor_org_id":2,"product_variant_id":1,"product_name":{"en":"Example"},"unit_price":"50.00","quantity":1}]}`, "checkout.wallet_on_website"},
		{"standalone_bundle", `{"branch_id":3,"payment_method":"cod","items":[{"vendor_org_id":2,"offer_product_id":9,"product_name":{"en":"Bundle"},"unit_price":"50.00","quantity":1}]}`, "checkout.unverifiable_on_website"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &apiGuardRepo{}
			response := postJSON(t, newAuthedRouter(repo), "/api/v1/commerce/checkout", tc.body)
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), tc.code) || repo.created != 0 {
				t.Fatalf("status=%d created=%d body=%s", response.Code, repo.created, response.Body.String())
			}
		})
	}
}
