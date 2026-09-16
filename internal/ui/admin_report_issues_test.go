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

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestReportIssuesFlow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)
	handler.RegisterPreApprovalRoutes(r)
	handler.RegisterApprovedSharedRoutes(r)
	handler.RegisterCustomerSharedRoutes(r)
	handler.RegisterVendorSharedRoutes(r)
	handler.RegisterPublicRoutes(r)

	t.Run("Customer can reach GET /report-issue", func(t *testing.T) {
		actor := &authctx.Actor{
			UserID:         20,
			OrganizationID: 200,
			OrgType:        "customer",
			OrgStatus:      "approved",
			Scope:          rbac.ScopePharmacy,
		}
		ctx := authctx.WithActor(context.Background(), *actor)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/report-issue", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("GET /report-issue returned status %d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("Vendor can reach GET /report-issue", func(t *testing.T) {
		actor := &authctx.Actor{
			UserID:         10,
			OrganizationID: 100,
			OrgType:        "vendor",
			OrgStatus:      "approved",
			Scope:          rbac.ScopeVendor,
		}
		ctx := authctx.WithActor(context.Background(), *actor)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/report-issue", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("GET /report-issue for vendor returned status %d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("Admin with workflow.issue.view can reach GET /admin/report-issues", func(t *testing.T) {
		actor := &authctx.Actor{
			UserID:      1,
			IsStaff:     true,
			Role:        "admin",
			Permissions: []string{"workflow.issue.view"},
		}
		ctx := authctx.WithActor(context.Background(), *actor)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/admin/report-issues", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("GET /admin/report-issues returned status %d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("User without workflow.issue.view gets redirected from GET /admin/report-issues", func(t *testing.T) {
		actor := &authctx.Actor{
			UserID:      5,
			IsStaff:     true,
			Role:        "clerk",
			Permissions: []string{"catalog.product.view"},
		}
		ctx := authctx.WithActor(context.Background(), *actor)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/admin/report-issues", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Errorf("GET /admin/report-issues without perm returned status %d, want %d", rr.Code, http.StatusSeeOther)
		}
	})

	t.Run("Admin with workflow.issue.update can post status update", func(t *testing.T) {
		actor := &authctx.Actor{
			UserID:      1,
			IsStaff:     true,
			Role:        "admin",
			Permissions: []string{"workflow.issue.update"},
		}
		ctx := authctx.WithActor(context.Background(), *actor)

		form := url.Values{}
		form.Set("status", "resolved")
		form.Set("response_notes", "تم حل المشكلة وتحديث البيانات")

		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/admin/report-issues/1/status", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// Expect redirect back (SeeOther 303)
		if rr.Code != http.StatusSeeOther {
			t.Errorf("POST /admin/report-issues/1/status returned status %d, want %d", rr.Code, http.StatusSeeOther)
		}
	})

	t.Run("Admin without workflow.issue.update gets redirected for post status update", func(t *testing.T) {
		actor := &authctx.Actor{
			UserID:      1,
			IsStaff:     true,
			Role:        "admin",
			Permissions: []string{"workflow.issue.view"}, // only view, not update
		}
		ctx := authctx.WithActor(context.Background(), *actor)

		form := url.Values{}
		form.Set("status", "resolved")

		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/admin/report-issues/1/status", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Errorf("POST /admin/report-issues/1/status without update perm returned %d, want %d", rr.Code, http.StatusSeeOther)
		}
	})
}
