package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
)

// Batch 3 (S06): a session that must change its password reaches only the
// account password page and the two password form endpoints.
func TestPasswordChangeRedirect(t *testing.T) {
	cases := []struct {
		name     string
		sess     *identity.Session
		path     string
		wantURL  string
		wantHeld bool
	}{
		{"unflagged session is never held", &identity.Session{OrgType: "vendor"}, "/vendor/orders", "", false},
		{"flagged vendor is sent to its sessions page", &identity.Session{MustChangePassword: true, OrgType: "vendor"}, "/vendor/orders", "/account/password", true},
		{"flagged pharmacy is sent to its sessions page", &identity.Session{MustChangePassword: true, OrgType: "customer"}, "/customer/cart", "/account/password", true},
		{"account password page stays reachable", &identity.Session{MustChangePassword: true, OrgType: "vendor"}, "/account/password", "", false},
		{"sessions page is no longer the target", &identity.Session{MustChangePassword: true, OrgType: "vendor"}, "/vendor/sessions", "/account/password", true},
		{"vendor password form stays reachable", &identity.Session{MustChangePassword: true, OrgType: "vendor"}, "/vendor/password", "", false},
		{"pharmacy password form stays reachable", &identity.Session{MustChangePassword: true, OrgType: "customer"}, "/customer/password", "", false},
		{"nil session is not held", nil, "/vendor/orders", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, held := passwordChangeRedirect(tc.sess, tc.path)
			if held != tc.wantHeld || got != tc.wantURL {
				t.Fatalf("passwordChangeRedirect(%s) = (%q, %v), want (%q, %v)", tc.path, got, held, tc.wantURL, tc.wantHeld)
			}
		})
	}
}

// The notification badge poll on the password page must not redirect: it would
// reload the page and wipe the form. Other held requests still redirect.
func TestHoldForPasswordChange(t *testing.T) {
	sess := &identity.Session{MustChangePassword: true, OrgType: "customer"}
	cases := []struct {
		name     string
		path     string
		hx       bool
		sess     *identity.Session
		wantHeld bool
		wantCode int
		wantHXR  string
	}{
		{"poll gets 204 and no redirect", "/notifications/unread-badge", true, sess, true, http.StatusNoContent, ""},
		{"htmx navigation is redirected", "/customer/cart", true, sess, true, http.StatusUnauthorized, "/account/password"},
		{"browser navigation is redirected", "/customer/cart", false, sess, true, http.StatusSeeOther, ""},
		{"password page passes", "/account/password", false, sess, false, http.StatusOK, ""},
		{"unflagged poll passes", "/notifications/unread-badge", true, &identity.Session{}, false, http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, tc.path, nil)
			if tc.hx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			if got := holdForPasswordChange(rec, req, tc.sess); got != tc.wantHeld {
				t.Fatalf("held = %v, want %v", got, tc.wantHeld)
			}
			if rec.Code != tc.wantCode {
				t.Fatalf("status %d, want %d", rec.Code, tc.wantCode)
			}
			if got := rec.Header().Get("HX-Redirect"); got != tc.wantHXR {
				t.Fatalf("HX-Redirect %q, want %q", got, tc.wantHXR)
			}
		})
	}
}
