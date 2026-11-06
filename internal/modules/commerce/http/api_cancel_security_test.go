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

type apiCancelRepo struct {
	happyRepo
	order       commerce.Order
	transitions int
}

func (r *apiCancelRepo) GetOrderByID(context.Context, int64) (*commerce.Order, error) {
	return &r.order, nil
}
func (r *apiCancelRepo) UpdateOrderStatus(context.Context, int64, commerce.OrderStatus, commerce.OrderStatusHistory) error {
	r.transitions++
	return nil
}
func TestAPICancellationRejectsUnsafeOrders(t *testing.T) {
	cases := []struct {
		name  string
		order commerce.Order
	}{
		{"shipped shipment", commerce.Order{ID: 1, CustomerID: 1, Status: commerce.StatusPending, Shipments: []*commerce.OrderShipment{{Status: commerce.StatusShipped}}}},
	}
	for _, tc := range cases {
		for _, route := range []struct{ path, body string }{{"/api/v1/commerce/orders/1/cancel", `{}`}, {"/api/v1/commerce/orders/1/status", `{"status":"cancelled"}`}} {
			t.Run(tc.name+route.path, func(t *testing.T) {
				repo := &apiCancelRepo{order: tc.order}
				actor := authctx.Actor{UserID: 1, OrganizationID: 7, OrgType: "pharmacy"}
				req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, route.path, strings.NewReader(route.body))
				req.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				newRouterWithActor(repo, actor).ServeHTTP(response, req)
				if response.Code < 400 || repo.transitions != 0 {
					t.Fatalf("status=%d transitions=%d body=%s", response.Code, repo.transitions, response.Body.String())
				}
			})
		}
	}
}
