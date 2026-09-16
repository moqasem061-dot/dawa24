package rbac_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// مندوب التوصيل as a role, and إدارة الشحنات as the one screen it opens.
//
// The rules held here are the ones a future edit is most likely to break by
// accident: that a courier's holding stays narrow, that the two courier
// actions stay separate from each other, and that the role does not leak into
// the pharmacy dashboard, where nobody does that job.

func TestCourierRoleIsSuppliersOnly(t *testing.T) {
	role, ok := rbac.OrganizationRole("org_courier")
	if !ok {
		t.Fatal("org_courier is not declared")
	}
	if !role.SeededIn(rbac.ScopeVendor) {
		t.Error("org_courier is not seeded into supplier companies")
	}
	if role.SeededIn(rbac.ScopePharmacy) {
		t.Error("org_courier is seeded into pharmacies, which employ no delivery representatives")
	}

	vendorRoles := rbac.OrganizationRolesFor(rbac.ScopeVendor)
	if !containsRole(vendorRoles, "org_courier") {
		t.Error("a supplier is not seeded with org_courier")
	}
	pharmacyRoles := rbac.OrganizationRolesFor(rbac.ScopePharmacy)
	if containsRole(pharmacyRoles, "org_courier") {
		t.Error("a pharmacy is seeded with org_courier")
	}
	// Every other starter role is seeded into both, and must stay that way:
	// this is a narrowing for one role, not a new per-scope role system.
	if len(vendorRoles) != len(pharmacyRoles)+1 {
		t.Errorf("supplier has %d starter roles and pharmacy %d; expected exactly one more",
			len(vendorRoles), len(pharmacyRoles))
	}
}

func containsRole(roles []rbac.SystemRole, key string) bool {
	for _, r := range roles {
		if r.Key == key {
			return true
		}
	}
	return false
}

// TestCourierHoldingIsNarrow. A مندوب carries parcels. They must not, by
// holding that role, also be able to read the company's sales, its catalogue
// or its money.
func TestCourierHoldingIsNarrow(t *testing.T) {
	role, _ := rbac.OrganizationRole("org_courier")
	held := rbac.NewSet(rbac.GrantsFor(role, rbac.ScopeVendor))

	for _, want := range []string{"vendor.delivery.view", "vendor.delivery.update"} {
		if !held.Has(want) {
			t.Errorf("a delivery representative does not hold %q", want)
		}
	}
	for _, forbidden := range []string{
		"vendor.dashboard.view",  // the company's sales
		"vendor.delivery.assign", // deciding whose round a parcel is on
		"vendor.order.view",      // أوامر التوريد
		"vendor.product.view",
		"vendor.wallet.view",
		"vendor.team.view",
	} {
		if held.Has(forbidden) {
			t.Errorf("a delivery representative holds %q, which is not their job", forbidden)
		}
	}
}

// TestDeliveryPortalIsTheCourierWholeSidebar. Their landing page and their
// navigation are the same one screen, so the section has to survive filtering
// down to a holding of two keys.
func TestDeliveryPortalIsTheCourierWholeSidebar(t *testing.T) {
	role, _ := rbac.OrganizationRole("org_courier")
	held := rbac.NewSet(rbac.GrantsFor(role, rbac.ScopeVendor))

	var links []string
	for _, sec := range rbac.VisibleNav(rbac.ScopeVendor, held) {
		for _, item := range sec.Items {
			if item.AlwaysVisible {
				continue
			}
			links = append(links, item.Href)
		}
	}
	if len(links) == 0 {
		t.Fatal("a delivery representative sees an empty sidebar")
	}
	if links[0] != "/vendor/delivery" {
		t.Errorf("the first link a courier sees is %q, want /vendor/delivery", links[0])
	}
	for _, href := range links {
		switch href {
		case "/vendor/delivery", "/vendor/sessions", "/notifications", "/vendor/mfa":
			continue
		default:
			t.Errorf("a delivery representative is offered %q", href)
		}
	}
}

// TestDispatchGrantIsSeparateFromDelivering. Assigning work and doing it are
// different jobs; a starter role that held both would make the split
// meaningless the day anyone used it.
func TestDispatchGrantIsSeparateFromDelivering(t *testing.T) {
	manager, _ := rbac.OrganizationRole("org_manager")
	held := rbac.NewSet(rbac.GrantsFor(manager, rbac.ScopeVendor))
	if !held.Has("vendor.delivery.assign") {
		t.Error("a supplier's manager cannot assign parcels to a delivery representative")
	}
	// .assign implies sight of the board and of the orders the parcels came
	// from, so a dispatcher is never sent to a page they cannot open.
	if !held.Has("vendor.delivery.view") {
		t.Error("assigning does not imply seeing the dispatch board")
	}
	if !held.Has("vendor.order.view") {
		t.Error("assigning does not imply seeing the supply orders the parcels come from")
	}

	warehouse, _ := rbac.OrganizationRole("org_warehouse")
	if !rbac.NewSet(rbac.GrantsFor(warehouse, rbac.ScopeVendor)).Has("vendor.delivery.assign") {
		t.Error("a warehouse keeper, who hands parcels over in person, cannot assign them")
	}
}

// TestDeliveryKeysAreVendorOnly. The pharmacy dashboard must not offer them:
// a pharmacy receives parcels, it does not dispatch them.
func TestDeliveryKeysAreVendorOnly(t *testing.T) {
	c := rbac.Default()
	for _, key := range []string{"vendor.delivery.view", "vendor.delivery.update", "vendor.delivery.assign"} {
		p, ok := c.Lookup(key)
		if !ok {
			t.Errorf("%q is not declared", key)
			continue
		}
		if !p.InScope(rbac.ScopeVendor) {
			t.Errorf("%q is not grantable on the supplier dashboard", key)
		}
		if p.InScope(rbac.ScopePharmacy) || p.InScope(rbac.ScopeAdmin) {
			t.Errorf("%q is grantable outside the supplier dashboard", key)
		}
	}
}

// TestTenantOrgTypesMatchTenantScopeFor holds the SQL-facing type list to the
// function every other caller resolves a dashboard through.
func TestTenantOrgTypesMatchTenantScopeFor(t *testing.T) {
	seen := map[string]bool{}
	for _, scope := range []rbac.Scope{rbac.ScopeVendor, rbac.ScopePharmacy} {
		types := rbac.TenantOrgTypes(scope)
		if len(types) == 0 {
			t.Errorf("%s maps to no organization type", scope)
		}
		for _, ty := range types {
			got, ok := rbac.TenantScopeFor(ty)
			if !ok || got != scope {
				t.Errorf("TenantOrgTypes(%s) lists %q, which resolves to %q", scope, ty, got)
			}
			if seen[ty] {
				t.Errorf("organization type %q is claimed by two dashboards", ty)
			}
			seen[ty] = true
		}
	}
	// The spellings live rows carry, per the note in permission.go.
	for _, ty := range []string{"vendor", "supplier", "company", "agency",
		"customer", "pharmacy", "chain_pharmacy", "individual"} {
		if !seen[ty] {
			t.Errorf("organization type %q is not claimed by any dashboard", ty)
		}
	}
}
