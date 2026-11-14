package ui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/features"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
)

func erpSwitchedOff(t *testing.T) {
	t.Helper()
	t.Cleanup(features.SetGlobal(features.NewStatic(map[string]bool{features.KeyERPEnabled: false})))
}

// With the integration switched off at runtime, every vendor and pharmacy
// integration route answers 404, GET and POST alike, exactly as if it did not
// exist; switched back on, the same routes are reached again with no restart.
func TestErpUIRoutesFollowRuntimeSwitch(t *testing.T) {
	h, _ := erpPageFixture()
	routes := []struct {
		name, method, path string
		register           func(r chi.Router)
	}{
		{"vendor page", "GET", "/vendor/integrations", func(r chi.Router) { h.registerVendorRoutes(r) }},
		{"vendor activity", "GET", "/vendor/integrations/activity", func(r chi.Router) { h.registerVendorRoutes(r) }},
		{"vendor review", "GET", "/vendor/integrations/review", func(r chi.Router) { h.registerVendorRoutes(r) }},
		{"vendor create key", "POST", "/vendor/integrations/keys", func(r chi.Router) { h.registerVendorRoutes(r) }},
		{"vendor pause", "POST", "/vendor/integrations/pause", func(r chi.Router) { h.registerVendorRoutes(r) }},
		{"customer page", "GET", "/customer/integrations", func(r chi.Router) { h.registerCustomerRoutes(r) }},
		{"customer create key", "POST", "/customer/integrations/keys", func(r chi.Router) { h.registerCustomerRoutes(r) }},
	}
	serve := func(register func(chi.Router), method, path string) int {
		r := chi.NewRouter()
		register(r)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w.Code
	}
	for _, tc := range routes {
		t.Run(tc.name, func(t *testing.T) {
			if got := serve(tc.register, tc.method, tc.path); got != http.StatusSeeOther {
				t.Fatalf("on: status=%d, want 303 (handler reached)", got)
			}
			restore := features.SetGlobal(features.NewStatic(map[string]bool{features.KeyERPEnabled: false}))
			off := serve(tc.register, tc.method, tc.path)
			restore()
			if off != http.StatusNotFound {
				t.Fatalf("off: status=%d, want 404", off)
			}
			if got := serve(tc.register, tc.method, tc.path); got != http.StatusSeeOther {
				t.Fatalf("on again: status=%d, want 303", got)
			}
		})
	}
}

// The staff integrations page stays reachable while the integration is off and
// says so.
func TestErpAdminPageShowsOffBanner(t *testing.T) {
	h, _ := erpPageFixture()
	staff := erpPageActor(1, true)
	staff.IsStaff = true
	staff.Role = "super_admin"
	get := func() (int, string) {
		req := httptest.NewRequest("GET", "/admin/integrations", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), staff))
		w := httptest.NewRecorder()
		h.ErpAdminKeysPage(w, req)
		return w.Code, w.Body.String()
	}
	if code, body := get(); code != 200 || strings.Contains(body, `id="erp-off-banner"`) {
		t.Fatalf("on: status=%d banner=%v, want 200 without banner", code, strings.Contains(body, `id="erp-off-banner"`))
	}
	erpSwitchedOff(t)
	code, body := get()
	if code != 200 || !strings.Contains(body, `id="erp-off-banner"`) || !strings.Contains(body, "/admin/settings?tab=features") {
		t.Fatalf("off: status=%d, want 200 with the off banner and a link to the switch", code)
	}
}

// While the integration is off the checkout neither asks for a customer code
// nor accepts one.
func TestCheckoutIgnoresErpCodesWhileSwitchedOff(t *testing.T) {
	h, _ := erpPageFixture()
	if !h.erpOn(context.Background()) {
		t.Fatal("erpOn is false with the service present and the switch on")
	}
	erpSwitchedOff(t)
	if h.erpOn(context.Background()) {
		t.Fatal("erpOn is true while the switch is off")
	}
	if f := h.erpMissingCodeFailure(context.Background(), authctx.Actor{UserID: 5}, nil); f != nil {
		t.Errorf("checkout is blocked on an ERP code while off: %+v", f)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/checkout/erp-customer-code", strings.NewReader("vendor_org_id=1&organization_number=X"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), erpPageActor(2, true)))
	h.CheckoutErpCodeSubmit(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("code submit status=%d while off, want 404", w.Code)
	}
}

type switchCall struct {
	enabled bool
	actor   int64
}

func stubSetERPFlag(t *testing.T, err error) *[]switchCall {
	t.Helper()
	var calls []switchCall
	prev := setERPFlag
	setERPFlag = func(_ context.Context, enabled bool, actorID int64) error {
		calls = append(calls, switchCall{enabled, actorID})
		return err
	}
	t.Cleanup(func() { setERPFlag = prev })
	return &calls
}

func switchRouter(h *UIHandler, actor *authctx.Actor) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if actor != nil {
				req = req.WithContext(authctx.WithActor(req.Context(), *actor))
			}
			next.ServeHTTP(w, req)
		})
	})
	h.registerAdminSettingsRoutes(r)
	return r
}

func postSwitch(handler http.Handler, enabled string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/admin/settings/erp-integration", strings.NewReader(url.Values{"enabled": {enabled}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestAdminERPSwitchSubmit(t *testing.T) {
	h, _ := erpPageFixture()
	staff := authctx.Actor{UserID: 9, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}

	t.Run("persists the flag with the actor", func(t *testing.T) {
		calls := stubSetERPFlag(t, nil)
		w := postSwitch(switchRouter(h, &staff), "false")
		if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "/admin/settings") {
			t.Fatalf("status=%d location=%q, want a redirect to the settings page", w.Code, w.Header().Get("Location"))
		}
		if len(*calls) != 1 || (*calls)[0] != (switchCall{false, 9}) {
			t.Fatalf("calls=%v, want one call (off, actor 9)", *calls)
		}
		postSwitch(switchRouter(h, &staff), "true")
		if len(*calls) != 2 || (*calls)[1] != (switchCall{true, 9}) {
			t.Fatalf("calls=%v, want the second call to turn it on", *calls)
		}
	})
	t.Run("a staff member without the settings permission is refused", func(t *testing.T) {
		calls := stubSetERPFlag(t, nil)
		limited := authctx.Actor{UserID: 10, IsStaff: true, Role: "moderator", Permissions: []string{"platform.setting.view"}}
		w := postSwitch(switchRouter(h, &limited), "false")
		if len(*calls) != 0 {
			t.Fatalf("the flag was written for an actor without platform.setting.update (status %d)", w.Code)
		}
	})
	t.Run("a tenant user is refused", func(t *testing.T) {
		calls := stubSetERPFlag(t, nil)
		tenant := erpPageActor(1, true)
		postSwitch(switchRouter(h, &tenant), "false")
		if len(*calls) != 0 {
			t.Fatal("the flag was written for a vendor owner")
		}
	})
	t.Run("anonymous is sent to login", func(t *testing.T) {
		calls := stubSetERPFlag(t, nil)
		postSwitch(switchRouter(h, nil), "false")
		if len(*calls) != 0 {
			t.Fatal("the flag was written without a session")
		}
	})
	t.Run("a garbage value changes nothing", func(t *testing.T) {
		calls := stubSetERPFlag(t, nil)
		postSwitch(switchRouter(h, &staff), "maybe")
		if len(*calls) != 0 {
			t.Fatal("the flag was written for an unparsable value")
		}
	})
	t.Run("a storage failure is reported, not hidden", func(t *testing.T) {
		stubSetERPFlag(t, errors.New("db down"))
		w := postSwitch(switchRouter(h, &staff), "false")
		if loc := w.Header().Get("Location"); w.Code != http.StatusSeeOther || !strings.Contains(loc, "notice=error") {
			t.Fatalf("status=%d location=%q, want an error notice", w.Code, loc)
		}
	})
	t.Run("CSRF is enforced", func(t *testing.T) {
		calls := stubSetERPFlag(t, nil)
		guarded := httpx.CSRF(true)(switchRouter(h, &staff))
		w := postSwitch(guarded, "false")
		if w.Code != http.StatusForbidden || len(*calls) != 0 {
			t.Fatalf("status=%d calls=%v, want 403 and no write without a CSRF token", w.Code, *calls)
		}
	})
}

func TestAdminSettingsCardIsOneSwitchNotAFlagList(t *testing.T) {
	h, _ := erpPageFixture()
	staff := authctx.Actor{UserID: 9, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}
	get := func() string {
		w := httptest.NewRecorder()
		h.AdminSettingsPage(w, httptest.NewRequest("GET", "/admin/settings", nil).WithContext(authctx.WithActor(context.Background(), staff)))
		return w.Body.String()
	}
	body := get()
	for _, want := range []string{`id="erp-switch-card"`, `action="/admin/settings/erp-integration"`, `name="enabled" value="false"`, "data-confirm="} {
		if !strings.Contains(body, want) {
			t.Errorf("on: settings page lacks %s", want)
		}
	}
	erpSwitchedOff(t)
	body = get()
	if !strings.Contains(body, `name="enabled" value="true"`) || strings.Contains(body, `name="enabled" value="false"`) {
		t.Error("off: the card should offer to turn the integration on, not off")
	}
	if strings.Contains(body, "erp.enabled") || strings.Contains(body, "/admin/settings/features/toggle") {
		t.Error("the card leaks the raw flag key or the removed toggle route")
	}
}
