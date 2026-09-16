package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminFullUserAndRolesRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous GET /admin/full-user redirects to login",
			path:       "/admin/full-user",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin GET /admin/users returns 200",
			path:   "/admin/users",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/full-user returns 301",
			path:   "/admin/full-user",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/customer-list returns 301",
			path:   "/admin/customer-list",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/vendor-list returns 301",
			path:   "/admin/vendor-list",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/roles returns 200",
			path:   "/admin/roles",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/want-delete returns 301",
			path:   "/admin/want-delete",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.actor != nil {
				ctx = authctx.WithActor(ctx, *tt.actor)
			}

			req, _ := http.NewRequestWithContext(ctx, tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("path %s expected status %d, got %d", tt.path, tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestActivitiesAndAuditLogsRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)
	handler.RegisterCustomerRoutes(r)
	handler.RegisterVendorRoutes(r)

	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous GET /customer/activities redirects to login",
			path:       "/customer/activities",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Customer GET /customer/activities returns 200",
			path:   "/customer/activities",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 100,
				Role:           "pharmacy_admin",
				Permissions:    []string{"pharmacy.team.view", "pharmacy.dashboard.view"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "Anonymous GET /vendor/activities redirects to login",
			path:       "/vendor/activities",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Vendor GET /vendor/activities returns 200",
			path:   "/vendor/activities",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         20,
				OrganizationID: 200,
				Role:           "vendor_admin",
				Permissions:    []string{"vendor.team.view", "vendor.activity.view"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/employee-activities returns 200",
			path:   "/admin/employee-activities",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/employee-activities with filters returns 200",
			path:   "/admin/employee-activities?q=test&org_id=1&user_id=2&action=offer",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/audit returns 301",
			path:   "/admin/audit",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/full-activity-logs returns 301",
			path:   "/admin/full-activity-logs",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/developers?tab=audit returns 200",
			path:   "/admin/developers?tab=audit",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.actor != nil {
				ctx = authctx.WithActor(ctx, *tt.actor)
			}

			req, _ := http.NewRequestWithContext(ctx, tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("path %s expected status %d, got %d", tt.path, tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestAdminUserEditAndPasswordRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous POST /admin/users/10/edit redirects to login",
			path:       "/admin/users/10/edit",
			method:     "POST",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Staff without identity.user.update POST /admin/users/10/edit redirects unauthorized",
			path:   "/admin/users/10/edit",
			method: "POST",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "staff",
				Permissions: []string{"identity.user.view"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:       "Anonymous POST /admin/users/10/password redirects to login",
			path:       "/admin/users/10/password",
			method:     "POST",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Staff without identity.user.update POST /admin/users/10/password redirects unauthorized",
			path:   "/admin/users/10/password",
			method: "POST",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "staff",
				Permissions: []string{"identity.user.view"},
			},
			wantStatus: http.StatusSeeOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.actor != nil {
				ctx = authctx.WithActor(ctx, *tt.actor)
			}

			req, _ := http.NewRequestWithContext(ctx, tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("path %s expected status %d, got %d", tt.path, tt.wantStatus, rr.Code)
			}
		})
	}
}
