package http

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
)

// Batch 3 (S06): a session that must change its password reaches only the
// sessions page and its password form.
func TestPasswordChangeRedirect(t *testing.T) {
	cases := []struct {
		name     string
		sess     *identity.Session
		path     string
		wantURL  string
		wantHeld bool
	}{
		{"unflagged session is never held", &identity.Session{OrgType: "vendor"}, "/vendor/orders", "", false},
		{"flagged vendor is sent to its sessions page", &identity.Session{MustChangePassword: true, OrgType: "vendor"}, "/vendor/orders", "/vendor/sessions", true},
		{"flagged pharmacy is sent to its sessions page", &identity.Session{MustChangePassword: true, OrgType: "customer"}, "/customer/cart", "/customer/sessions", true},
		{"vendor sessions page stays reachable", &identity.Session{MustChangePassword: true, OrgType: "vendor"}, "/vendor/sessions", "", false},
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

func TestHasKey(t *testing.T) {
	keys := []string{"vendor.session.view", "vendor.order.view"}
	if !hasKey(keys, "vendor.session.view") {
		t.Fatal("present key not found")
	}
	if hasKey(keys, "vendor.session") {
		t.Fatal("partial key must not match")
	}
}
