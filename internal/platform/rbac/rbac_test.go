package rbac_test

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// TestCatalogBuilds is the guard on the declarations themselves: Default()
// panics on a duplicate key, an unknown group, a dangling implication or a
// scope mismatch, so simply reaching it proves the catalogue is coherent.
func TestCatalogBuilds(t *testing.T) {
	c := rbac.Default()
	if len(c.Permissions()) == 0 {
		t.Fatal("catalogue is empty")
	}
	for _, s := range rbac.Scopes() {
		if len(c.PermissionsFor(s)) == 0 {
			t.Errorf("scope %s declares no permissions", s)
		}
		if len(c.Matrix(s)) == 0 {
			t.Errorf("scope %s produces an empty role editor", s)
		}
	}
}

// TestEverySidebarItemHasAGrantablePermission is the rule the old sidebars
// broke. The admin shell gated six links on keys such as
// "notifications.notification.view" that identity.permissions never held, so
// no role could be granted them and the links were invisible to everyone but
// super_admin — which reads as "the feature is missing", not "you lack access".
func TestEverySidebarItemHasAGrantablePermission(t *testing.T) {
	c := rbac.Default()
	for _, scope := range rbac.Scopes() {
		inScope := map[string]bool{}
		for _, p := range c.PermissionsFor(scope) {
			inScope[p.Key] = true
		}
		for _, sec := range rbac.Nav(scope) {
			for _, item := range sec.Items {
				// An AlwaysVisible item is the one deliberate exception: it is
				// about the caller, not about the company, so there is no
				// permission to grant and inventing one would only create a
				// role that can be configured to lock someone out of their own
				// password. TestAlwaysVisibleItemsAreTheSameEverywhere holds
				// that category to its own rule.
				if item.AlwaysVisible {
					if item.Perm != "" {
						t.Errorf("%s: sidebar item %q is AlwaysVisible and also names permission %q; it must be one or the other",
							scope, item.Key, item.Perm)
					}
					continue
				}
				if item.Perm == "" {
					t.Errorf("%s: sidebar item %q declares no permission", scope, item.Key)
					continue
				}
				for _, key := range append([]string{item.Perm}, item.Also...) {
					if !c.Known(key) {
						t.Errorf("%s: sidebar item %q names undeclared permission %q", scope, item.Key, key)
						continue
					}
					if !inScope[key] {
						t.Errorf("%s: sidebar item %q names permission %q, which is not grantable in that scope",
							scope, item.Key, key)
					}
				}
			}
		}
	}
}

// TestNavKeysAreUniqueWithinAScope catches two links claiming one activeNav
// value, which would highlight both and make the active state meaningless.
func TestNavKeysAreUniqueWithinAScope(t *testing.T) {
	for _, scope := range rbac.Scopes() {
		seen := map[string]string{}
		for _, sec := range rbac.Nav(scope) {
			for _, item := range sec.Items {
				if prev, dup := seen[item.Key]; dup {
					t.Errorf("%s: nav key %q claimed by both %q and %q", scope, item.Key, prev, sec.Key)
				}
				seen[item.Key] = sec.Key
			}
		}
	}
}

// TestRestrictIsTheCompanyBoundary. A vendor owner who posts a role form
// naming platform permissions must get a role without them. Filtering the
// checkboxes shown in the editor is a convenience; this is the control.
func TestRestrictIsTheCompanyBoundary(t *testing.T) {
	c := rbac.Default()
	forged := []string{
		"vendor.order.view",       // legitimately theirs
		"platform.setting.update", // another dashboard entirely
		"identity.user.delete",    // platform staff only
		"pharmacy.order.create",   // a different tenant dashboard
		"catalog.product.delete",  // admin catalogue, not vendor items
		"totally.made.up",         // not declared at all
	}
	got := c.Restrict(forged, rbac.ScopeVendor)
	for _, k := range got {
		if !strings.HasPrefix(k, "vendor.") {
			t.Errorf("Restrict leaked %q into the vendor scope", k)
		}
	}
	if !contains(got, "vendor.order.view") {
		t.Error("Restrict dropped a permission the vendor legitimately holds")
	}
}

// TestActionsImplyTheirPage. A role that may edit a page it cannot open is a
// role whose owner will file a bug, so an action expands to its page.
func TestActionsImplyTheirPage(t *testing.T) {
	c := rbac.Default()
	for _, scope := range rbac.Scopes() {
		for _, p := range c.PermissionsFor(scope) {
			if p.Kind != rbac.KindAction || len(p.Implies) == 0 {
				continue
			}
			expanded := c.Expand([]string{p.Key})
			for _, want := range p.Implies {
				if !contains(expanded, want) {
					t.Errorf("%s: %q does not expand to its implied %q", scope, p.Key, want)
				}
			}
		}
	}
}

// TestOwnerHoldsEverythingInScopeAndNothingOutsideIt.
func TestOwnerHoldsEverythingInScopeAndNothingOutsideIt(t *testing.T) {
	c := rbac.Default()
	owner, ok := rbac.OrganizationRole("org_owner")
	if !ok {
		t.Fatal("org_owner is not declared")
	}
	for _, scope := range []rbac.Scope{rbac.ScopeVendor, rbac.ScopePharmacy} {
		grants := rbac.GrantsFor(owner, scope)
		if len(grants) != len(c.KeysFor(scope)) {
			t.Errorf("%s: owner holds %d of %d permissions", scope, len(grants), len(c.KeysFor(scope)))
		}
		set := rbac.NewSet(grants)
		other := rbac.ScopeAdmin
		for _, k := range c.KeysFor(other) {
			if p, _ := c.Lookup(k); p.InScope(scope) {
				continue // legitimately shared
			}
			if set.Has(k) {
				t.Errorf("%s owner holds out-of-scope permission %q", scope, k)
			}
		}
	}
}

// TestSetMatching covers the wildcard rules the middleware relies on.
func TestSetMatching(t *testing.T) {
	s := rbac.NewSet([]string{"vendor.order.view", "catalog.*"})
	cases := []struct {
		want string
		ok   bool
	}{
		{"vendor.order.view", true},
		{"vendor.order.update", false},
		{"catalog.product.view", true},
		{"catalog.brand.delete", true},
		{"catalogue.product.view", false}, // prefix must stop at the dot
		{"commerce.order.view", false},
	}
	for _, tc := range cases {
		if got := s.Has(tc.want); got != tc.ok {
			t.Errorf("Has(%q) = %v, want %v", tc.want, got, tc.ok)
		}
	}

	all := rbac.NewSet([]string{rbac.Wildcard})
	if !all.Has("anything.at.all") {
		t.Error("the wildcard grant does not satisfy an arbitrary permission")
	}
	empty := rbac.NewSet(nil)
	if empty.Has("vendor.order.view") {
		t.Error("an empty holding satisfied a requirement")
	}
	if !empty.Has("") {
		t.Error("an ungated requirement was denied")
	}
}

// TestSystemRoleGrantsAreDeclared. A starter role naming a permission that no
// longer exists would seed a company with dead grants.
func TestSystemRoleGrantsAreDeclared(t *testing.T) {
	c := rbac.Default()
	for _, r := range rbac.PlatformRoles() {
		for _, k := range r.Permissions {
			if !c.Known(k) {
				t.Errorf("platform role %q grants undeclared permission %q", r.Key, k)
			}
		}
	}
	for _, r := range rbac.OrganizationRoles() {
		for _, scope := range []rbac.Scope{rbac.ScopeVendor, rbac.ScopePharmacy} {
			for _, k := range rbac.GrantsFor(r, scope) {
				if !c.Known(k) {
					t.Errorf("org role %q in %s grants undeclared permission %q", r.Key, scope, k)
				}
			}
		}
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// TestAlwaysVisibleItemsAreTheSameEverywhere is the rule that keeps
// NavItem.AlwaysVisible from becoming a way to smuggle a company page past the
// permission registry.
//
// An item every member sees whatever they hold can only be a page about the
// caller themselves. Such a page is the same page for a pharmacist, a supplier
// and a platform administrator, so the same key must resolve to the same URL in
// every scope that declares it. A supplier-only destination marked
// AlwaysVisible would fail here rather than quietly appearing in the pharmacy
// sidebar.
func TestAlwaysVisibleItemsAreTheSameEverywhere(t *testing.T) {
	hrefs := map[string]string{}
	scopes := map[string]string{}
	for _, scope := range rbac.Scopes() {
		for _, sec := range rbac.Nav(scope) {
			for _, item := range sec.Items {
				if !item.AlwaysVisible {
					continue
				}
				if item.Href == "" {
					t.Errorf("%s: always-visible item %q has no href", scope, item.Key)
					continue
				}
				if seen, ok := hrefs[item.Key]; ok && seen != item.Href {
					t.Errorf("always-visible item %q points at %q in %s but %q in %s; it must be one destination",
						item.Key, seen, scopes[item.Key], item.Href, scope)
					continue
				}
				hrefs[item.Key] = item.Href
				scopes[item.Key] = string(scope)
			}
		}
	}
	if len(hrefs) == 0 {
		t.Fatal("no always-visible navigation items; account settings should be one")
	}
	if hrefs["account_settings"] != "/settings" {
		t.Errorf("account settings should be always-visible at /settings, got %q", hrefs["account_settings"])
	}
}

// TestSharedCapabilitiesAreDeclaredInBothScopes.
//
// A Capability is a promise that one page is grantable on both dashboards. The
// promise is only kept if both keys exist and each is grantable in its own
// scope — otherwise RequiredKeys hands the gate a key nobody can hold, and the
// page is closed to that dashboard with nothing in the role editor to explain
// why. That is the failure the whole catalogue exists to make impossible, in
// the one place a single declaration spans two scopes.
func TestSharedCapabilitiesAreDeclaredInBothScopes(t *testing.T) {
	c := rbac.Default()
	all := append(rbac.BuyingCapabilities(), rbac.SharedCompanyCapabilities()...)
	if len(all) == 0 {
		t.Fatal("no shared capabilities are declared")
	}
	for _, cap := range all {
		for _, tc := range []struct {
			scope rbac.Scope
			key   string
		}{
			{rbac.ScopePharmacy, cap.Pharmacy},
			{rbac.ScopeVendor, cap.Vendor},
		} {
			if key := cap.KeyFor(tc.scope); key != tc.key {
				t.Errorf("capability %q: KeyFor(%s) = %q, want %q", cap.Name, tc.scope, key, tc.key)
			}
			if tc.key == "" {
				t.Errorf("capability %q declares no key for %s", cap.Name, tc.scope)
				continue
			}
			p, ok := c.Lookup(tc.key)
			if !ok {
				t.Errorf("capability %q names undeclared permission %q", cap.Name, tc.key)
				continue
			}
			if !p.InScope(tc.scope) {
				t.Errorf("capability %q names %q for %s, which is not grantable there",
					cap.Name, tc.key, tc.scope)
			}
		}
		if got := rbac.RequiredKeys(rbac.ScopeAdmin, cap); len(got) != 0 {
			t.Errorf("capability %q resolves to %v on the admin dashboard; a shared page is a tenant page",
				cap.Name, got)
		}
	}
}

// TestCapabilityNamesAreUnique catches two capabilities claiming one name,
// which would make a test failure name the wrong page.
func TestCapabilityNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, cap := range append(rbac.BuyingCapabilities(), rbac.SharedCompanyCapabilities()...) {
		if seen[cap.Name] {
			t.Errorf("duplicate capability name %q", cap.Name)
		}
		seen[cap.Name] = true
	}
}

// TestVendorPurchasingSection pins the section this feature exists to add.
//
// A supplier restocks from other distributors, and does it on the same six
// screens a pharmacy buys through. The names and the order are the product
// decision; the permissions are what make each one hideable.
func TestVendorPurchasingSection(t *testing.T) {
	var buying *rbac.NavSection
	for _, sec := range rbac.Nav(rbac.ScopeVendor) {
		if sec.Key == "buying" {
			s := sec
			buying = &s
			break
		}
	}
	if buying == nil {
		t.Fatal("the vendor sidebar has no purchasing section")
	}
	if buying.NameAr != "شراء المنتجات" {
		t.Errorf("the section is labelled %q", buying.NameAr)
	}

	want := []struct{ key, href, perm string }{
		{"orders", "/orders", "vendor.buying.order.view"},
		{"purchase-request", "/customer/purchase-request", "vendor.buying.purchase_request.view"},
		{"followed", "/suppliers/followed", "vendor.buying.supplier.follow"},
		{"suppliers", "/customer/suppliers", "vendor.buying.supplier.view"},
		{"catalog", "/customer/catalog", "vendor.buying.catalog.view"},
		{"offers", "/customer/offers", "vendor.buying.offer.view"},
	}
	if len(buying.Items) != len(want) {
		t.Fatalf("the section has %d items, want %d", len(buying.Items), len(want))
	}
	for i, w := range want {
		got := buying.Items[i]
		if got.Key != w.key || got.Href != w.href || got.Perm != w.perm {
			t.Errorf("item %d is {%s %s %s}, want {%s %s %s}",
				i, got.Key, got.Href, got.Perm, w.key, w.href, w.perm)
		}
	}

	// A supplier holding only their selling dashboard sees none of it: the
	// section is a grant, not a consequence of being a supplier.
	selling := rbac.NewSet([]string{
		"vendor.dashboard.view", "vendor.product.view",
		"vendor.order.view", "vendor.offer.view", "vendor.wallet.view",
	})
	for _, sec := range rbac.VisibleNav(rbac.ScopeVendor, selling) {
		if sec.Key == "buying" {
			t.Error("the purchasing section is shown to a supplier holding no buying grant")
		}
	}
}

// TestVendorSellingAndBuyingNavKeysAreDistinct.
//
// أوامر التوريد (orders placed WITH this supplier) and طلباتي (orders this
// supplier placed) are two pages in one sidebar. One activeNav value must
// highlight one of them, or both light up and the active state means nothing.
func TestVendorSellingAndBuyingNavKeysAreDistinct(t *testing.T) {
	byKey := map[string]string{}
	for _, sec := range rbac.Nav(rbac.ScopeVendor) {
		for _, item := range sec.Items {
			byKey[item.Key] = item.Href
		}
	}
	pairs := []struct{ selling, buying, sellingHref, buyingHref string }{
		{"supply_orders", "orders", "/vendor/orders", "/orders"},
		{"supply_offers", "offers", "/vendor/offers", "/customer/offers"},
	}
	for _, p := range pairs {
		if byKey[p.selling] != p.sellingHref {
			t.Errorf("nav key %q points at %q, want %q", p.selling, byKey[p.selling], p.sellingHref)
		}
		if byKey[p.buying] != p.buyingHref {
			t.Errorf("nav key %q points at %q, want %q", p.buying, byKey[p.buying], p.buyingHref)
		}
	}
}
