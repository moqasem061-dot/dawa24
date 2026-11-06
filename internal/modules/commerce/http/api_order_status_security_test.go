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

type orderStatusSecurityRepo struct {
	happyRepo
	orderCalls          int
	shipmentCalls       []int64
	paymentMethod       string
	orderStatus         commerce.OrderStatus
	otherShipmentStatus commerce.OrderStatus
}

func (r *orderStatusSecurityRepo) GetOrderByID(_ context.Context, id int64) (*commerce.Order, error) {
	status := r.orderStatus
	if status == "" {
		status = commerce.StatusPending
	}
	other := r.otherShipmentStatus
	if other == "" {
		other = commerce.StatusPending
	}
	return &commerce.Order{ID: id, CustomerID: 100, PaymentMethod: r.paymentMethod, Status: status, Shipments: []*commerce.OrderShipment{{ID: 11, OrderID: id, OrganizationID: 10, Status: commerce.StatusPending}, {ID: 22, OrderID: id, OrganizationID: 20, Status: other}}}, nil
}

func TestAPIVendorCancellationHonorsOrderAndOtherShipments(t *testing.T) {
	for _, repo := range []*orderStatusSecurityRepo{{otherShipmentStatus: commerce.StatusShipped}} {
		actor := authctx.Actor{UserID: 101, OrganizationID: 10, OrgType: "vendor", Permissions: []string{"vendor.order.update"}}
		request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/commerce/orders/1/status", strings.NewReader(`{"status":"cancelled"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		newRouterWithActor(repo, actor).ServeHTTP(response, request)
		if response.Code < 400 || len(repo.shipmentCalls) != 0 {
			t.Fatalf("status=%d calls=%v", response.Code, repo.shipmentCalls)
		}
	}
}

func TestAPIVendorCannotConfirmWalletWithoutCharging(t *testing.T) {
	repo := &orderStatusSecurityRepo{paymentMethod: "wallet"}
	actor := authctx.Actor{UserID: 101, OrganizationID: 10, OrgType: "vendor", Permissions: []string{"vendor.order.update"}}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/commerce/orders/1/status", strings.NewReader(`{"status":"confirmed"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	newRouterWithActor(repo, actor).ServeHTTP(response, request)
	if response.Code < 400 || len(repo.shipmentCalls) != 0 {
		t.Fatalf("status=%d shipment calls=%v", response.Code, repo.shipmentCalls)
	}
}

type walletShipmentRepo struct {
	happyRepo
	transitions int
}

func (r *walletShipmentRepo) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{ID: id, OrderID: 1, OrganizationID: 10, Status: commerce.StatusPending}, nil
}
func (r *walletShipmentRepo) GetOrderByID(_ context.Context, id int64) (*commerce.Order, error) {
	return &commerce.Order{ID: id, CustomerID: 100, Status: commerce.StatusPending, PaymentMethod: "wallet"}, nil
}
func (r *walletShipmentRepo) UpdateShipmentStatus(context.Context, int64, commerce.OrderStatus, commerce.OrderStatus, commerce.OrderStatusHistory) error {
	r.transitions++
	return nil
}

func TestAPIShipmentCannotConfirmOrCancelWalletWithoutPayment(t *testing.T) {
	for _, status := range []string{"confirmed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			repo := &walletShipmentRepo{}
			actor := authctx.Actor{UserID: 101, OrganizationID: 10, OrgType: "vendor", Permissions: []string{"vendor.order.update"}}
			request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/commerce/shipments/11/status", strings.NewReader(`{"status":"`+status+`"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			newRouterWithActor(repo, actor).ServeHTTP(response, request)
			if response.Code < 400 || repo.transitions != 0 {
				t.Fatalf("status=%d transitions=%d", response.Code, repo.transitions)
			}
		})
	}
}
func (r *orderStatusSecurityRepo) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{ID: id, OrderID: 1, OrganizationID: 10, Status: commerce.StatusPending}, nil
}
func (r *orderStatusSecurityRepo) UpdateShipmentStatus(_ context.Context, id int64, _, _ commerce.OrderStatus, _ commerce.OrderStatusHistory) error {
	r.shipmentCalls = append(r.shipmentCalls, id)
	return nil
}
func (r *orderStatusSecurityRepo) UpdateOrderStatus(_ context.Context, _ int64, _ commerce.OrderStatus, _ commerce.OrderStatusHistory) error {
	r.orderCalls++
	return nil
}
func (r *orderStatusSecurityRepo) CancelAPIVendorShipment(_ context.Context, request commerce.APIVendorCancelRequest) error {
	r.shipmentCalls = append(r.shipmentCalls, request.ShipmentID)
	return nil
}

func TestAPIVendorCancellationAffectsOnlyOwnShipment(t *testing.T) {
	repo := &orderStatusSecurityRepo{}
	actor := authctx.Actor{UserID: 101, OrganizationID: 10, OrgType: "vendor", Permissions: []string{"vendor.order.update"}}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/commerce/orders/1/status", strings.NewReader(`{"status":"cancelled"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	newRouterWithActor(repo, actor).ServeHTTP(response, request)
	if response.Code != http.StatusOK || repo.orderCalls != 0 || len(repo.shipmentCalls) != 1 || repo.shipmentCalls[0] != 11 {
		t.Fatalf("status=%d order calls=%d shipment calls=%v", response.Code, repo.orderCalls, repo.shipmentCalls)
	}
}
func TestAPIVendorOrderStatusScope(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "permission_required", true: "own_shipment_only"}[allowed], func(t *testing.T) {
			repo := &orderStatusSecurityRepo{}
			actor := authctx.Actor{UserID: 101, OrganizationID: 10, OrgType: "vendor"}
			if allowed {
				actor.Permissions = []string{"vendor.order.update"}
			}
			request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/commerce/orders/1/status", strings.NewReader(`{"status":"confirmed"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			newRouterWithActor(repo, actor).ServeHTTP(response, request)
			if !allowed && response.Code != http.StatusForbidden {
				t.Fatalf("got %d, want 403", response.Code)
			}
			if allowed && (response.Code != http.StatusOK || repo.orderCalls != 0 || len(repo.shipmentCalls) != 1 || repo.shipmentCalls[0] != 11) {
				t.Fatalf("status=%d order calls=%d shipments=%v body=%s", response.Code, repo.orderCalls, repo.shipmentCalls, response.Body.String())
			}
		})
	}
}
