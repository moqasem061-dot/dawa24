package rbac

import "testing"

func TestScopedKey(t *testing.T) {
	c := Default()
	cases := []struct {
		key   string
		scope Scope
		want  string
	}{
		// A supplier buying: the buyer-side keys become its buying keys.
		{"pharmacy.order.view", ScopeVendor, "vendor.buying.order.view"},
		{"pharmacy.order.create", ScopeVendor, "vendor.buying.order.create"},
		{"pharmacy.purchase_request.view", ScopeVendor, "vendor.buying.purchase_request.view"},
		{"pharmacy.wallet.view", ScopeVendor, "vendor.wallet.view"},
		// And back again for a pharmacy.
		{"vendor.buying.order.view", ScopePharmacy, "pharmacy.order.view"},
		{"vendor.wallet.view", ScopePharmacy, "pharmacy.wallet.view"},
		{"vendor.organization.view", ScopePharmacy, "pharmacy.organization.view"},
		{"vendor.document.view", ScopePharmacy, "pharmacy.document.view"},
		{"vendor.subscription.view", ScopePharmacy, "pharmacy.subscription.view"},
		// Already right for the dashboard.
		{"pharmacy.order.view", ScopePharmacy, "pharmacy.order.view"},
		{"vendor.order.view", ScopeVendor, "vendor.order.view"},
		// No counterpart: unchanged, so only owners and staff pass.
		{"vendor.ad.view", ScopePharmacy, "vendor.ad.view"},
		{"commerce.order.view", ScopeVendor, "commerce.order.view"},
		{"", ScopeVendor, ""},
		// The platform dashboard is never translated.
		{"pharmacy.order.view", ScopeAdmin, "pharmacy.order.view"},
	}
	for _, tc := range cases {
		if got := c.ScopedKey(tc.key, tc.scope); got != tc.want {
			t.Errorf("ScopedKey(%q, %s) = %q, want %q", tc.key, tc.scope, got, tc.want)
		}
	}
}

// Every key a translation produces must be grantable on the target dashboard,
// or the notification would reach nobody but the owner again.
func TestScopedKeyResultsAreGrantable(t *testing.T) {
	c := Default()
	for _, p := range c.Permissions() {
		for _, s := range []Scope{ScopeVendor, ScopePharmacy} {
			got := c.ScopedKey(p.Key, s)
			if got == p.Key {
				continue
			}
			q, ok := c.Lookup(got)
			if !ok || !q.InScope(s) {
				t.Errorf("ScopedKey(%q, %s) = %q, which is not grantable there", p.Key, s, got)
			}
		}
	}
}
