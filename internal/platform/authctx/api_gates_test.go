package authctx_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/authctx/authctxtest"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

func TestRequireAPIPermission(t *testing.T) {
	gate := authctx.RequireAPIPermission("commerce.admin")

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	superAdmin := authctxtest.ActorForRole("super_admin", rbac.ScopeAdmin)
	supportUser := authctxtest.ActorForRole("support", rbac.ScopeAdmin)
	customerUser := authctxtest.ActorForRole("user", rbac.ScopePharmacy)

	// Explicit synthetic actor testing exact staff permission grant
	staffWithPerm := authctxtest.SyntheticActor(10, true, "commerce.admin")

	tests := []struct {
		name       string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous request returns 401",
			actor:      nil,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Non-staff customer returns 403",
			actor:      &customerUser,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "Staff user lacking permission returns 403",
			actor:      &supportUser,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "Staff user with required permission returns 200",
			actor:      &staffWithPerm,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Super admin with owner grant returns 200",
			actor:      &superAdmin,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/admin/commerce/orders", nil)
			if tt.actor != nil {
				req = req.WithContext(authctx.WithActor(req.Context(), *tt.actor))
			}
			rec := httptest.NewRecorder()
			gate(okHandler).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequireAPITenantPermission(t *testing.T) {
	gate := authctx.RequireAPITenantPermission("vendor.order.update")

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	superAdmin := authctxtest.ActorForRole("super_admin", rbac.ScopeAdmin)
	vendorManager := authctxtest.ActorForRole("org_manager", rbac.ScopeVendor)
	vendorAccountant := authctxtest.ActorForRole("org_accountant", rbac.ScopeVendor)

	userNoOrg := authctx.Actor{
		UserID:         2,
		OrganizationID: 0,
		Role:           "vendor",
		Permissions:    []string{"vendor.order.update"},
	}

	tests := []struct {
		name       string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous request returns 401",
			actor:      nil,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Staff member calling tenant API returns 403",
			actor:      &superAdmin,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "User with no organization returns 403",
			actor:      &userNoOrg,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "Tenant user lacking permission returns 403",
			actor:      &vendorAccountant,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "Tenant user with permission returns 200",
			actor:      &vendorManager,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/vendor/orders/1/status", nil)
			if tt.actor != nil {
				req = req.WithContext(authctx.WithActor(req.Context(), *tt.actor))
			}
			rec := httptest.NewRecorder()
			gate(okHandler).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequireApprovedAPI(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	gate := authctx.RequireApproved(log)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	approvedActor := authctxtest.ActorForRole("org_manager", rbac.ScopeVendor)

	pendingActor := authctxtest.ActorForRole("org_manager", rbac.ScopeVendor)
	pendingActor.OrgStatus = "pending"

	rejectedActor := authctxtest.ActorForRole("org_manager", rbac.ScopeVendor)
	rejectedActor.OrgStatus = "rejected"

	suspendedActor := authctxtest.ActorForRole("org_manager", rbac.ScopeVendor)
	suspendedActor.OrgStatus = "suspended"

	staffActor := authctxtest.ActorForRole("super_admin", rbac.ScopeAdmin)

	tests := []struct {
		name       string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous request returns 401",
			actor:      nil,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Pending org on API returns 403",
			actor:      &pendingActor,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "Rejected org on API returns 403",
			actor:      &rejectedActor,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "Suspended org on API returns 403",
			actor:      &suspendedActor,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "Approved org on API returns 200",
			actor:      &approvedActor,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Staff member on API returns 200",
			actor:      &staffActor,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/commerce/orders", nil)
			if tt.actor != nil {
				req = req.WithContext(authctx.WithActor(req.Context(), *tt.actor))
			}
			rec := httptest.NewRecorder()
			gate(okHandler).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
