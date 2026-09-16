package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	commerceHttp "github.com/muhiya/dawa24-store/internal/modules/commerce/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/authctx/authctxtest"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

type shipmentTestRepo struct {
	happyRepo
	shipmentOrgID int64
}

func (r shipmentTestRepo) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{
		ID:             id,
		OrganizationID: r.shipmentOrgID,
		Status:         commerce.StatusPending,
	}, nil
}

func (r shipmentTestRepo) ListShipmentsByVendor(_ context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	return []*commerce.OrderShipment{
		{ID: 1, OrganizationID: vendorOrgID, ShipmentNumber: "SH-1"},
	}, nil
}

func newRouterWithActor(repo commerce.Repository, actor authctx.Actor) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := commerce.NewService(repo, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := authctx.WithActor(r.Context(), actor)
			if actor.OrganizationID > 0 {
				ctx = database.WithTenant(ctx, actor.OrganizationID)
			} else {
				ctx = database.WithTenant(ctx, 1)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	commerceHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

func TestShipmentOwnershipAndSpoofing(t *testing.T) {
	// Shipment owned by Vendor Org 10
	repo := shipmentTestRepo{shipmentOrgID: 10}

	vendorOrg10Member := authctxtest.ActorForRole("org_manager", rbac.ScopeVendor)
	vendorOrg10Member.UserID = 101
	vendorOrg10Member.OrganizationID = 10

	vendorOrg20Member := authctxtest.ActorForRole("org_manager", rbac.ScopeVendor)
	vendorOrg20Member.UserID = 201
	vendorOrg20Member.OrganizationID = 20

	customerActor := authctxtest.ActorForRole("org_owner", rbac.ScopePharmacy)
	customerActor.UserID = 301
	customerActor.OrganizationID = 30

	staffActor := authctxtest.ActorForRole("super_admin", rbac.ScopeAdmin)
	staffActor.UserID = 901

	t.Run("Task 1: ListVendorShipments spoofing another vendor_id returns 403", func(t *testing.T) {
		router := newRouterWithActor(repo, vendorOrg10Member)
		req := httptest.NewRequest("GET", "/api/v1/commerce/vendor/shipments?vendor_id=20", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("got status %d, want 403 Forbidden (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Task 1: ListVendorShipments for own org succeeds", func(t *testing.T) {
		router := newRouterWithActor(repo, vendorOrg10Member)
		req := httptest.NewRequest("GET", "/api/v1/commerce/vendor/shipments", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want 200 OK (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Task 1: Non-vendor customer listing shipments returns 403", func(t *testing.T) {
		router := newRouterWithActor(repo, customerActor)
		req := httptest.NewRequest("GET", "/api/v1/commerce/vendor/shipments?vendor_id=10", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("got status %d, want 403 Forbidden", rec.Code)
		}
	})

	t.Run("Task 1: Staff listing shipments with explicit vendor_id succeeds", func(t *testing.T) {
		router := newRouterWithActor(repo, staffActor)
		req := httptest.NewRequest("GET", "/api/v1/commerce/vendor/shipments?vendor_id=10", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want 200 OK", rec.Code)
		}
	})

	t.Run("Task 2: TransitionShipmentStatus by non-owner vendor returns 403", func(t *testing.T) {
		router := newRouterWithActor(repo, vendorOrg20Member)
		req := httptest.NewRequest("POST", "/api/v1/commerce/shipments/1/status", strings.NewReader(`{"status":"confirmed"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("got status %d, want 403 Forbidden (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Task 2: TransitionShipmentStatus by owning vendor with permission succeeds", func(t *testing.T) {
		router := newRouterWithActor(repo, vendorOrg10Member)
		req := httptest.NewRequest("POST", "/api/v1/commerce/shipments/1/status", strings.NewReader(`{"status":"confirmed"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want 200 OK (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Task 2: TransitionShipmentStatus by staff succeeds", func(t *testing.T) {
		router := newRouterWithActor(repo, staffActor)
		req := httptest.NewRequest("POST", "/api/v1/commerce/shipments/1/status", strings.NewReader(`{"status":"confirmed"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want 200 OK (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Task 3: GetShipment by non-owner vendor returns 403", func(t *testing.T) {
		router := newRouterWithActor(repo, vendorOrg20Member)
		req := httptest.NewRequest("GET", "/api/v1/commerce/shipments/1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("got status %d, want 403 Forbidden (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Task 3: GetShipment by owning vendor succeeds", func(t *testing.T) {
		router := newRouterWithActor(repo, vendorOrg10Member)
		req := httptest.NewRequest("GET", "/api/v1/commerce/shipments/1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want 200 OK (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Task 3: GetShipment by staff succeeds", func(t *testing.T) {
		router := newRouterWithActor(repo, staffActor)
		req := httptest.NewRequest("GET", "/api/v1/commerce/shipments/1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want 200 OK (body: %s)", rec.Code, rec.Body.String())
		}
	})
}
