package ui_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// The quota screen's wiring: routes registered, permissions enforced, and the
// three POSTs reachable.
//
// The repository behaviour is proved by the integration tests against a real
// database; what can only be proved here is that /vendor/quotas exists at all,
// that a supplier without vendor.quota.manage cannot reach the actions, and
// that a pharmacy cannot reach any of it.

func newQuotaRouter(actor *authctx.Actor) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	stubAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if actor == nil {
				next.ServeHTTP(w, req)
				return
			}
			ctx := authctx.WithActor(req.Context(), *actor)
			if actor.OrganizationID > 0 {
				ctx = database.WithTenant(ctx, actor.OrganizationID)
			}
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}

	r := chi.NewRouter()
	r.Group(func(vendorRouter chi.Router) {
		vendorRouter.Use(stubAuth)
		vendorRouter.Use(authctx.RequireVendor(logger))
		vendorRouter.Use(authctx.RequireApproved(logger))
		handler.RegisterVendorRoutes(vendorRouter)
	})
	return r
}

func quotaActor(perms ...string) *authctx.Actor {
	return &authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Role:           "vendor",
		Permissions:    perms,
	}
}

func TestVendorQuotaRoutes(t *testing.T) {
	full := newQuotaRouter(quotaActor("vendor.*"))
	viewOnly := newQuotaRouter(quotaActor("vendor.quota.view", "vendor.dashboard.view"))
	pharmacy := newQuotaRouter(&authctx.Actor{
		UserID: 2, OrganizationID: 20, OrgType: "customer",
		OrgStatus: "approved", Role: "customer", Permissions: []string{"pharmacy.*"},
	})

	t.Run("a supplier with the permission opens the page", func(t *testing.T) {
		rec := httptest.NewRecorder()
		full.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/vendor/quotas", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		// With no commerce service wired the page still has to render its own
		// chrome rather than fall over: a nil service is what a partially
		// configured deployment looks like.
		if !strings.Contains(rec.Body.String(), "حصص الفروع") {
			t.Error("expected the quota page title in the body")
		}
	})

	t.Run("the view permission alone opens the page", func(t *testing.T) {
		rec := httptest.NewRecorder()
		viewOnly.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/vendor/quotas", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
	})

	// The three actions are gated on vendor.quota.manage, so someone who may
	// read the figures cannot hand out more allowance.
	for _, path := range []string{
		"/vendor/quotas/limit",
		"/vendor/quotas/release",
		"/vendor/quotas/release/undo",
	} {
		t.Run("view-only is refused "+path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path,
				strings.NewReader(url.Values{"variant_id": {"1"}, "branch_id": {"2"}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			viewOnly.ServeHTTP(rec, req)
			if rec.Code == http.StatusOK {
				t.Errorf("%s must not be reachable without vendor.quota.manage (got %d)", path, rec.Code)
			}
		})
	}

	t.Run("a pharmacy cannot reach the supplier's quota screen", func(t *testing.T) {
		rec := httptest.NewRecorder()
		pharmacy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/vendor/quotas", nil))
		if rec.Code == http.StatusOK {
			t.Errorf("a customer must not open /vendor/quotas (got %d)", rec.Code)
		}
	})
}
