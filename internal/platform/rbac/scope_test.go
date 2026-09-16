package rbac_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// TestLegacyOrganizationTypesResolveToADashboard.
//
// The bug this exists to prevent, exactly as it appeared: a supplier opened
// /vendor/roles and got the pharmacy shell — "بوابة الصيدلية المعتمدة" in the
// header — with a completely empty sidebar.
//
// Two facts combined. Most supplier rows carry the legacy type "supplier"
// rather than "vendor", a spelling inherited from the Laravel schema. And the
// shell chose its frame with `actor.OrgType == "vendor"`, a literal string
// comparison, so every such account fell through to the pharmacy branch. The
// sidebar was then empty for a sound reason: it renders only items the caller
// holds a *pharmacy* permission for, and a supplier holds none.
//
// The lesson is in the shape of the failure. Nothing errored. The page
// rendered, in the wrong clothes, with no navigation — which reads as "the
// feature is broken" rather than "the type has two spellings".
func TestLegacyOrganizationTypesResolveToADashboard(t *testing.T) {
	cases := []struct {
		orgType string
		want    rbac.Scope
	}{
		// Canonical spellings.
		{"vendor", rbac.ScopeVendor},
		{"customer", rbac.ScopePharmacy},
		// Legacy spellings that live in production rows today. "supplier" is
		// the one that broke the roles page.
		{"supplier", rbac.ScopeVendor},
		{"company", rbac.ScopeVendor},
		{"agency", rbac.ScopeVendor},
		{"pharmacy", rbac.ScopePharmacy},
		{"chain_pharmacy", rbac.ScopePharmacy},
		{"individual", rbac.ScopePharmacy},
	}

	for _, tc := range cases {
		t.Run(tc.orgType, func(t *testing.T) {
			got, ok := TenantScopeOrFail(t, tc.orgType)
			if !ok {
				t.Fatalf("organization type %q resolves to no dashboard; its members would see an empty sidebar", tc.orgType)
			}
			if got != tc.want {
				t.Errorf("scope = %q, want %q", got, tc.want)
			}
			// And the dashboard it resolves to must actually have navigation,
			// or the caller lands on a frame with nothing in it.
			if len(rbac.Nav(got)) == 0 {
				t.Errorf("scope %q declares no navigation", got)
			}
		})
	}

	// An unknown type resolves to nothing rather than defaulting. Guessing
	// would put an unrecognised company on some dashboard, which is how a
	// tenant ends up looking at the wrong one.
	if _, ok := rbac.TenantScopeFor("something_new"); ok {
		t.Error("an unrecognised organization type was assigned a dashboard")
	}
	if _, ok := rbac.TenantScopeFor(""); ok {
		t.Error("an empty organization type was assigned a dashboard")
	}
}

// TestNormalizeOrgTypeIsIdempotent. Normalizing an already-canonical value must
// not change it, or a second pass through the login path would drift.
func TestNormalizeOrgTypeIsIdempotent(t *testing.T) {
	for _, in := range []string{"vendor", "supplier", "customer", "pharmacy", "unknown", ""} {
		once := rbac.NormalizeOrgType(in)
		if twice := rbac.NormalizeOrgType(once); twice != once {
			t.Errorf("NormalizeOrgType(%q) = %q, then %q; normalization is not stable", in, once, twice)
		}
	}
}

// TestSupplierSeesNoPharmacyNavigationAndViceVersa.
//
// The separation the dashboards depend on: a company holding its whole own
// dashboard must still see nothing at all of the other one.
func TestSupplierSeesNoPharmacyNavigationAndViceVersa(t *testing.T) {
	vendorOwner := rbac.NewSet(rbac.Default().KeysFor(rbac.ScopeVendor))
	pharmacyOwner := rbac.NewSet(rbac.Default().KeysFor(rbac.ScopePharmacy))

	if got := countNavItems(rbac.VisibleNav(rbac.ScopeVendor, vendorOwner)); got == 0 {
		t.Error("a vendor owner sees no vendor navigation at all")
	}
	if got := countGrantedNavItems(rbac.VisibleNav(rbac.ScopePharmacy, vendorOwner)); got != 0 {
		t.Errorf("a vendor owner sees %d pharmacy navigation items", got)
	}
	if got := countGrantedNavItems(rbac.VisibleNav(rbac.ScopeAdmin, vendorOwner)); got != 0 {
		t.Errorf("a vendor owner sees %d admin navigation items", got)
	}

	if got := countNavItems(rbac.VisibleNav(rbac.ScopePharmacy, pharmacyOwner)); got == 0 {
		t.Error("a pharmacy owner sees no pharmacy navigation at all")
	}
	if got := countGrantedNavItems(rbac.VisibleNav(rbac.ScopeVendor, pharmacyOwner)); got != 0 {
		t.Errorf("a pharmacy owner sees %d vendor navigation items", got)
	}
	if got := countGrantedNavItems(rbac.VisibleNav(rbac.ScopeAdmin, pharmacyOwner)); got != 0 {
		t.Errorf("a pharmacy owner sees %d admin navigation items", got)
	}
}

// TestSupplierOnlyFeaturesAreNotOfferedToPharmacies.
//
// The discount comparison tool and the market-discount board read across
// suppliers' price lists. They are supplier features and must not appear in a
// pharmacy sidebar, nor in a pharmacy owner's role editor — which means the
// permission cannot exist in the pharmacy scope, not merely be hidden.
func TestSupplierOnlyFeaturesAreNotOfferedToPharmacies(t *testing.T) {
	c := rbac.Default()
	for _, key := range []string{"vendor.compare.use", "vendor.market_discounts.view"} {
		p, ok := c.Lookup(key)
		if !ok {
			t.Errorf("%q is not declared", key)
			continue
		}
		if p.InScope(rbac.ScopePharmacy) {
			t.Errorf("%q is grantable in the pharmacy scope; it is a supplier feature", key)
		}
	}
	// And nothing pharmacy-scoped points at those pages any more.
	for _, p := range c.PermissionsFor(rbac.ScopePharmacy) {
		if p.Nav == "compare" || p.Nav == "market-discounts" {
			t.Errorf("pharmacy permission %q still reveals the supplier-only %q item", p.Key, p.Nav)
		}
	}
	for _, section := range rbac.Nav(rbac.ScopePharmacy) {
		for _, item := range section.Items {
			if item.Key == "compare" || item.Key == "market-discounts" {
				t.Errorf("the pharmacy sidebar still lists the supplier-only item %q", item.Key)
			}
		}
	}
}

// countGrantedNavItems counts only the items a holding actually earns.
//
// An always-visible item — account settings — appears in every scope by
// construction and says nothing about whether one dashboard is leaking into
// another, which is the only thing the separation tests are asking about.
func countGrantedNavItems(sections []rbac.NavSection) int {
	n := 0
	for _, sec := range sections {
		for _, item := range sec.Items {
			if !item.AlwaysVisible {
				n++
			}
		}
	}
	return n
}

func countNavItems(sections []rbac.NavSection) int {
	n := 0
	for _, s := range sections {
		n += len(s.Items)
	}
	return n
}

// TenantScopeOrFail is a thin wrapper that keeps the table above readable.
func TenantScopeOrFail(t *testing.T, orgType string) (rbac.Scope, bool) {
	t.Helper()
	return rbac.TenantScopeFor(orgType)
}
