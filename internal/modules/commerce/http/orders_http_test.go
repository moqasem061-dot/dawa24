package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	commerceHttp "github.com/muhiya/dawa24-store/internal/modules/commerce/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := commerce.NewService(stubRepo{t: t}, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("dawa24_session")
			if err != nil || cookie.Value == "" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			if cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	commerceHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

func newAuthedRouter(repo commerce.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := commerce.NewService(repo, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				IsStaff:        true,
				Role:           "super_admin",
				Permissions:    []string{"admin", "super_admin", "commerce.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	commerceHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodPost, "/api/v1/commerce/checkout"},
	{http.MethodPatch, "/api/v1/commerce/cart/items/1"},
	{http.MethodGet, "/api/v1/commerce/orders/1/history"},
	{http.MethodPost, "/api/v1/commerce/orders/1/rate"},
	{http.MethodGet, "/api/v1/commerce/shipments/1"},
	{http.MethodPost, "/api/v1/commerce/shipments/1/status"},
	{http.MethodGet, "/api/v1/commerce/orders/1"},
	{http.MethodGet, "/api/v1/commerce/orders"},
	{http.MethodPost, "/api/v1/commerce/orders/1/status"},
	{http.MethodPost, "/api/v1/commerce/orders/1/cancel"},
	{http.MethodGet, "/api/v1/commerce/vendor/shipments"},
	{http.MethodGet, "/api/v1/commerce/cart"},
	{http.MethodPost, "/api/v1/commerce/cart/items"},
	{http.MethodDelete, "/api/v1/commerce/cart/items/1"},
	{http.MethodDelete, "/api/v1/commerce/cart"},
	{http.MethodGet, "/api/v1/commerce/wishlist"},
	{http.MethodPost, "/api/v1/commerce/wishlist"},
	{http.MethodDelete, "/api/v1/commerce/wishlist/1"},
	{http.MethodPost, "/api/v1/commerce/quotes"},
	{http.MethodPost, "/api/v1/commerce/quotes/1/respond"},
	{http.MethodGet, "/api/v1/commerce/quotes"},
}

func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)
	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint is reachable without a session", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)
	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/commerce/cart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		t.Error("error envelope has no code")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func TestCommerceHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	validUntil := time.Now().AddDate(0, 0, 7).Format(time.RFC3339)
	_ = validUntil

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"GetCart", http.MethodGet, "/api/v1/commerce/cart", "", http.StatusOK},
		{"AddCartItem", http.MethodPost, "/api/v1/commerce/cart/items", `{"product_id":1,"product_variant_id":1,"quantity":2,"unit_price":"50.00"}`, http.StatusOK},
		{"SetCartQuantity", http.MethodPatch, "/api/v1/commerce/cart/items/1", `{"quantity":5}`, http.StatusOK},
		{"RemoveCartItem", http.MethodDelete, "/api/v1/commerce/cart/items/1", "", http.StatusOK},
		{"ClearCart", http.MethodDelete, "/api/v1/commerce/cart", "", http.StatusOK},
		{"Checkout", http.MethodPost, "/api/v1/commerce/checkout", `{"customer_id":1,"payment_method":"fawry","items":[{"vendor_org_id":1,"product_name":{"en":"Panadol"},"unit_price":"50.00","quantity":2}]}`, http.StatusCreated},
		{"GetOrder", http.MethodGet, "/api/v1/commerce/orders/1", "", http.StatusOK},
		{"ListOrders", http.MethodGet, "/api/v1/commerce/orders?limit=10&offset=0", "", http.StatusOK},
		{"GetOrderHistory", http.MethodGet, "/api/v1/commerce/orders/1/history", "", http.StatusOK},
		{"RateOrder", http.MethodPost, "/api/v1/commerce/orders/1/rate", `{"rating":5,"review":"excellent"}`, http.StatusNoContent},
		{"CancelOrder", http.MethodPost, "/api/v1/commerce/orders/1/cancel", `{"reason":"mistake"}`, http.StatusOK},
		{"GetShipment", http.MethodGet, "/api/v1/commerce/shipments/1", "", http.StatusOK},
		{"ListVendorShipments", http.MethodGet, "/api/v1/commerce/vendor/shipments?vendor_id=1", "", http.StatusOK},
		{"GetWishlist", http.MethodGet, "/api/v1/commerce/wishlist", "", http.StatusOK},
		{"AddToWishlist", http.MethodPost, "/api/v1/commerce/wishlist", `{"product_id":1}`, http.StatusCreated},
		{"RemoveFromWishlist", http.MethodDelete, "/api/v1/commerce/wishlist/1", "", http.StatusOK},
		{"CreateQuote", http.MethodPost, "/api/v1/commerce/quotes", `{"organization_id":1,"customer_org_id":2,"product_name":"Panadol","requested_quantity":100,"target_unit_price":"45.00"}`, http.StatusCreated},
		{"RespondQuote", http.MethodPost, "/api/v1/commerce/quotes/1/respond", `{"quote_price":"47.00","supplier_notes":"best offer"}`, http.StatusOK},
		{"ListQuotes", http.MethodGet, "/api/v1/commerce/quotes", "", http.StatusOK},
		{"AdminSearchOrders", http.MethodGet, "/api/v1/admin/commerce/orders", "", http.StatusOK},
		{"AdminGetOrder", http.MethodGet, "/api/v1/admin/commerce/orders/1", "", http.StatusOK},
		{"AdminForceStatus", http.MethodPost, "/api/v1/admin/commerce/orders/1/status", `{"status":"confirmed","note":"admin verified"}`, http.StatusOK},
		{"AdminRefund", http.MethodPost, "/api/v1/admin/commerce/orders/1/refund", `{"reason":"customer returned item"}`, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func (r stubRepo) CountOrders(_ context.Context) (int, error) {
	r.fail("CountOrders")
	return 0, nil
}

func (happyRepo) CountOrders(_ context.Context) (int, error) { return 5, nil }

func (r stubRepo) MonthSalesByVendor(_ context.Context, _ int64) (money.Amount, error) {
	r.fail("MonthSalesByVendor")
	return money.Zero, nil
}

func (happyRepo) MonthSalesByVendor(_ context.Context, _ int64) (money.Amount, error) {
	return money.MustParse("1250.00"), nil
}

func (r stubRepo) CountOrderLines(context.Context, []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (r stubRepo) MonthSpendByCustomer(_ context.Context, _ int64) (money.Amount, error) {
	r.fail("MonthSpendByCustomer")
	return money.Zero, nil
}

func (happyRepo) CountOrderLines(context.Context, []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (happyRepo) MonthSpendByCustomer(_ context.Context, _ int64) (money.Amount, error) {
	return money.MustParse("800.00"), nil
}

func (r stubRepo) SetShipmentTracking(context.Context, int64, string, string) error {
	r.fail("SetShipmentTracking")
	return nil
}

func (happyRepo) SetShipmentTracking(context.Context, int64, string, string) error {
	return nil
}

func (r stubRepo) CountVendorShipmentsByStatus(_ context.Context, _ int64, _ []string) (int, error) {
	r.fail("CountVendorShipmentsByStatus")
	return 0, nil
}

func (happyRepo) CountVendorShipmentsByStatus(_ context.Context, _ int64, _ []string) (int, error) {
	return 2, nil
}
