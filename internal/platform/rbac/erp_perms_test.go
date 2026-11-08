package rbac_test

import (
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
	"testing"
)

func testERPAccess(t *testing.T, scope rbac.Scope, view, action string, implies bool) {
	t.Helper()
	c := rbac.Default()
	for _, key := range []string{view, action} {
		p, ok := c.Lookup(key)
		if !ok || !p.InScope(scope) {
			t.Errorf("%s is missing from %s", key, scope)
		}
	}
	if implies && !contains(c.Expand([]string{action}), view) {
		t.Errorf("%s does not imply %s", action, view)
	}
	found := false
	for _, section := range rbac.Nav(scope) {
		for _, item := range section.Items {
			if item.Key == "integrations" && item.Perm == view {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("%s integrations navigation missing", scope)
	}
}
func TestErpPermsVendor(t *testing.T) {
	testERPAccess(t, rbac.ScopeVendor, "vendor.integration.view", "vendor.integration.manage", true)
	c := rbac.Default()
	review, ok := c.Lookup("vendor.integration.review")
	if !ok || !review.InScope(rbac.ScopeVendor) {
		t.Fatal("vendor.integration.review is missing from the vendor scope")
	}
	// The review key is its own grant: neither manage nor view implies it, and
	// resolving review items is a separate screen a supplier can withhold.
	if contains(c.Expand([]string{"vendor.integration.manage"}), "vendor.integration.review") {
		t.Error("vendor.integration.manage must not imply vendor.integration.review")
	}
	if contains(c.Expand([]string{"vendor.integration.review"}), "vendor.integration.view") {
		t.Error("vendor.integration.review must not imply vendor.integration.view")
	}
}
func TestErpPermsPharmacy(t *testing.T) {
	testERPAccess(t, rbac.ScopePharmacy, "pharmacy.integration.view", "pharmacy.integration.manage", true)
}
func TestErpPermsAdmin(t *testing.T) {
	testERPAccess(t, rbac.ScopeAdmin, "platform.integration.view", "platform.integration.revoke", false)
}
