package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestErpUIRoutesFollowDisabledFlag proves SEC-006: with SetERP(nil) — the
// wiring's ERP_API_ENABLED=false posture — none of the three integration
// surfaces register any route, and with the service present they all do.
func TestErpUIRoutesFollowDisabledFlag(t *testing.T) {
	enabled, _ := erpPageFixture()
	disabled := NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, enabled.log)

	for _, tc := range []struct {
		name, method, path string
		register           func(h *UIHandler, r chi.Router)
	}{
		{"vendor page", "GET", "/vendor/integrations", func(h *UIHandler, r chi.Router) { h.registerVendorRoutes(r) }},
		{"vendor create", "POST", "/vendor/integrations/keys", func(h *UIHandler, r chi.Router) { h.registerVendorRoutes(r) }},
		{"customer page", "GET", "/customer/integrations", func(h *UIHandler, r chi.Router) { h.registerCustomerRoutes(r) }},
		{"customer create", "POST", "/customer/integrations/keys", func(h *UIHandler, r chi.Router) { h.registerCustomerRoutes(r) }},
		{"admin page", "GET", "/admin/integrations", func(h *UIHandler, r chi.Router) { h.RegisterAdminRoutes(r) }},
		{"admin revoke", "POST", "/admin/integrations/keys/1/revoke", func(h *UIHandler, r chi.Router) { h.RegisterAdminRoutes(r) }},
	} {
		t.Run(tc.name+"/disabled", func(t *testing.T) {
			r := chi.NewRouter()
			tc.register(disabled, r)
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Fatalf("disabled route %s %s status=%d, want 404", tc.method, tc.path, w.Code)
			}
		})
		t.Run(tc.name+"/enabled", func(t *testing.T) {
			r := chi.NewRouter()
			tc.register(enabled, r)
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			// No actor is in the request context, so reaching the handler at
			// all yields the login redirect (303): proof the route exists.
			if w.Code != http.StatusSeeOther {
				t.Fatalf("enabled route %s %s status=%d, want 303 (handler reached)", tc.method, tc.path, w.Code)
			}
		})
	}
}
