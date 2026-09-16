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

func TestPlatformPhase9Routes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)
	handler.RegisterPreApprovalRoutes(r)
	handler.RegisterApprovedSharedRoutes(r)
	handler.RegisterCustomerSharedRoutes(r)
	handler.RegisterVendorSharedRoutes(r)
	handler.RegisterPublicRoutes(r)

	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous GET /auth/2fa-challenge returns 404 (deleted theatre)",
			path:       "/auth/2fa-challenge",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "User GET /settings/security/2fa returns 404 (deleted theatre)",
			path:   "/settings/security/2fa",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 2,
				OrgType:        "customer",
				Permissions:    []string{"pharmacy.*"},
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "User GET /invoices/10/pdf returns 404 (deleted fake stub)",
			path:   "/invoices/10/pdf",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 2,
				OrgType:        "customer",
				Permissions:    []string{"pharmacy.*"},
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "User GET /report-issue returns 200",
			path:   "/report-issue",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 2,
				OrgType:        "customer",
				Permissions:    []string{"pharmacy.*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/session-plan redirects to /admin/plans",
			path:   "/admin/session-plan",
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
			name:   "Super admin GET /admin/report-issues returns 200",
			path:   "/admin/report-issues",
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
