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

func TestPromoPhase8Routes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)
	handler.RegisterVendorRoutes(r)
	handler.RegisterPublicRoutes(r)

	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:   "Super admin GET /admin/offers-packages returns 200",
			path:   "/admin/offers-packages",
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
			name:   "Super admin GET /admin/offers-packages/packages returns 200",
			path:   "/admin/offers-packages/packages",
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
			name:   "Super admin GET /admin/offers-packages/sponsorships returns 200",
			path:   "/admin/offers-packages/sponsorships",
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
			name:   "Super admin GET /admin/offer-sponsorships returns 301",
			path:   "/admin/offer-sponsorships",
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
			name:   "Super admin GET /admin/ads returns 301 (redirected to /admin/offers-packages?tab=ads)",
			path:   "/admin/ads",
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
			name:   "Vendor GET /vendor/offers-packages returns 200",
			path:   "/vendor/offers-packages",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
				Permissions:    []string{"vendor.*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/ads returns 200",
			path:   "/vendor/ads",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
				Permissions:    []string{"vendor.*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "Public GET /promotions/track-click/5 redirects safely",
			path:       "/promotions/track-click/5",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:       "Public GET /ads/click/2 redirects safely",
			path:       "/ads/click/2",
			method:     "GET",
			actor:      nil,
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
