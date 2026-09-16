package ui_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func newTestRouter(actor *authctx.Actor) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	// Mirrors cmd/server/routes.go, except session-based RequireAuth is
	// replaced by a stub that carries an optional test actor.
	stubAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if _, ok := authctx.From(req.Context()); ok {
				next.ServeHTTP(w, req)
				return
			}
			if actor != nil {
				next.ServeHTTP(w, req.WithContext(authctx.WithActor(req.Context(), *actor)))
				return
			}
			next.ServeHTTP(w, req)
		})
	}

	r := chi.NewRouter()
	r.Use(stubAuth)
	handler.RegisterPublicRoutes(r)
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireCustomer(logger))
		uiRouter.Use(authctx.RequireApproved(logger))
		handler.RegisterCustomerRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireBuyer(logger))
		uiRouter.Use(authctx.RequireApproved(logger))
		handler.RegisterBuyingRoutes(uiRouter)
		handler.RegisterSmartOrderRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireVendor(logger))
		uiRouter.Use(authctx.RequireApproved(logger))
		handler.RegisterVendorRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireStaff(logger))
		handler.RegisterAdminRoutes(uiRouter)
	})
	// The shared surface is three tiers in production and must be three here,
	// or this harness cannot catch a regression in the thing it exists to
	// check. Mounting all four registrars in one ungated group made every
	// Tier B and Tier C route reachable by any actor the test supplied,
	// including the pending and suspended ones.
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		handler.RegisterPreApprovalRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireApproved(logger))
		handler.RegisterApprovedSharedRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireCustomer(logger))
		handler.RegisterCustomerSharedRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireVendor(logger))
		handler.RegisterVendorSharedRoutes(uiRouter)
	})
	return r
}

func setupTestRouter() http.Handler {
	return newTestRouter(&authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		OrgStatus:   "approved",
		Permissions: []string{"*"},
	})
}

func TestPublicAndAuthPageRoutes(t *testing.T) {
	router := newTestRouter(nil)

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/"},
		{"GET", "/about"},
		{"GET", "/how-it-works"},
		{"GET", "/faq"},
		{"GET", "/contact"},
		{"GET", "/privacy"},
		{"GET", "/terms"},
		{"GET", "/auth/login"},
		{"GET", "/auth/register"},
		{"GET", "/auth/forgot"},
		{"GET", "/auth/reset?token=test-reset-tok"},
		{"GET", "/jobs"},
	}

	for _, route := range routes {
		t.Run(route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("%s %s returned status %d, want 200", route.method, route.path, rec.Code)
			}
		})
	}
}

// The public marketplace pages stay open to signed-out visitors on purpose,
// except /catalog, which is authenticated-only: it publishes supplier identity
// with net supply prices and stock, and the business requires a signed-in
// account to see it. Scraping of the remaining open pages is answered by
// internal/platform/antiscrape — see scrape_guard_test.go.
func TestPublicMarketplacePagesAreOpenToGuests(t *testing.T) {
	router := newTestRouter(nil)

	for _, path := range []string{"/offers", "/suppliers"} {
		t.Run("Guest_Allowed_"+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("GET %s as a guest returned %d, want 200", path, rec.Code)
			}
			if loc := rec.Header().Get("Location"); strings.Contains(loc, "/auth/login") {
				t.Errorf("GET %s as a guest redirected to %q; the login wall on the public pages was removed deliberately", path, loc)
			}
		})
	}
}

// /catalog (listing and detail) is authenticated-only: a signed-out visitor is
// sent to sign in instead of receiving any listing, capped or otherwise.
func TestCatalogRequiresSignIn(t *testing.T) {
	router := newTestRouter(nil)

	for _, path := range []string{"/catalog", "/catalog/1"} {
		t.Run("Guest_Blocked_"+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound && rec.Code != http.StatusTemporaryRedirect {
				t.Errorf("GET %s for guest returned status %d, want redirect to login", path, rec.Code)
			}
			if loc := rec.Header().Get("Location"); !strings.Contains(loc, "/auth/login") {
				t.Errorf("GET %s redirected to %q, want /auth/login redirect", path, loc)
			}
		})
	}
}

// The compare workspace is the opposite case and must stay gated: every page in
// it reads the caller's own uploaded price lists, so there is nothing to show a
// visitor who has not signed in.
func TestCompareWorkspaceStillRequiresSignIn(t *testing.T) {
	router := newTestRouter(nil)

	for _, path := range []string{"/compare", "/compare/results"} {
		t.Run("Guest_Blocked_"+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound && rec.Code != http.StatusTemporaryRedirect {
				t.Errorf("GET %s for guest returned status %d, want redirect to login", path, rec.Code)
			}
			if loc := rec.Header().Get("Location"); !strings.Contains(loc, "/auth/login") {
				t.Errorf("GET %s redirected to %q, want /auth/login redirect", path, loc)
			}
		})
	}
}

func TestHTMXPartialHeaderHandling(t *testing.T) {
	router := setupTestRouter()

	// The catalogue partial renders for whoever asks for it
	req := httptest.NewRequest("GET", "/catalog", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /catalog with HX-Request returned status %d, want 200", rec.Code)
	}
}

func TestAuthenticatedUIRoutesWithActor(t *testing.T) {
	actor := authctx.Actor{
		UserID:         100,
		OrganizationID: 200,
		Role:           "user",
		OrgType:        "customer",
		OrgStatus:      "approved",
		// The pharmacy dashboard is permission-gated now. This double stands
		// in for a company owner, who holds their whole dashboard.
		Permissions: []string{"pharmacy.*"},
	}
	router := newTestRouter(&actor)

	req := httptest.NewRequest("GET", "/cart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /cart with actor returned status %d, want 200", rec.Code)
	}
}

func TestFormActionRoutes(t *testing.T) {
	staffActor := &authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		OrgStatus:   "approved",
		Permissions: []string{"*"},
	}
	customerActor := &authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		OrgType:        "customer",
		OrgStatus:      "approved",
		Permissions:    []string{"pharmacy.*", "*"},
	}
	vendorActor := &authctx.Actor{
		UserID:         20,
		OrganizationID: 200,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Permissions:    []string{"vendor.*", "*"},
	}

	actionRoutes := []struct {
		method string
		path   string
		actor  *authctx.Actor
	}{
		{"POST", "/auth/logout", staffActor},
		{"GET", "/auth/logout", staffActor},
		{"POST", "/admin/settings", staffActor},
		{"POST", "/cart/add", customerActor},
		{"POST", "/cart/remove", customerActor},
		{"POST", "/checkout", customerActor},
		{"POST", "/notifications/123/read", staffActor},
		{"POST", "/vendor/variants/new", vendorActor},
		{"POST", "/vendor/variants/7/toggle-status", vendorActor},
		{"POST", "/vendor/products/activate-all", vendorActor},
		{"POST", "/vendor/orders/456/status", vendorActor},
		{"POST", "/admin/products/new", staffActor},
	}

	for _, route := range actionRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			router := newTestRouter(route.actor)
			req := httptest.NewRequest(route.method, route.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			// Actions perform redirects (303 See Other) or re-render form on validation error (422 Unprocessable Entity)
			if rec.Code != http.StatusSeeOther && rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s %s returned status %d, want %d or %d", route.method, route.path, rec.Code, http.StatusSeeOther, http.StatusUnprocessableEntity)
			}
		})
	}
}
