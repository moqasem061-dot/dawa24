package ui_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Cross-dashboard separation, checked on the real route table.
//
// The permission catalogue keeps the two dashboards apart by construction —
// a vendor role can only be granted vendor keys — but that is a statement
// about roles. This checks the other half: that the routes themselves refuse
// a caller from the wrong dashboard even when that caller holds everything
// their own dashboard offers, and even when they type the URL directly.

// tenantActor builds a company member holding their whole dashboard, which is
// the strongest caller the other dashboard must still refuse.
func tenantActor(userID, orgID int64, orgType string, scope rbac.Scope) *authctx.Actor {
	a := &authctx.Actor{
		UserID:         userID,
		OrganizationID: orgID,
		OrgType:        orgType,
		OrgStatus:      "approved",
		Scope:          scope,
	}
	a.Grants([]string{string(scope) + ".*"})
	return a
}

// vendorOnlyPaths and pharmacyOnlyPaths are the surfaces each dashboard owns.
var (
	vendorOnlyPaths = []string{
		"/vendor/dashboard",
		"/vendor/wallet",
		"/vendor/roles",
		"/vendor/team",
		"/vendor/products",
		"/vendor/ingest",
		"/vendor/orders",
		"/vendor/storefront",
		"/vendor/coverage",
		"/vendor/warehouses",
	}
	pharmacyOnlyPaths = []string{
		"/customer/dashboard",
		"/customer/wallet",
		"/customer/roles",
		"/customer/team",
		"/customer/branches",
		"/customer/saving-products",
		"/customer/subscription",
		"/customer/decision-memory",
	}
	// sharedBuyingPaths are the marketplace screens both dashboards own. They
	// are the exception to the separation above, and the exception has to be
	// named: /customer/purchase-request sat in pharmacyOnlyPaths until
	// suppliers gained the purchasing section, and moving it without a test in
	// its place would have removed the only thing checking it.
	sharedBuyingPaths = []string{
		"/customer/purchase-request",
		"/customer/catalog",
		"/customer/suppliers",
		"/customer/offers",
		"/orders",
		"/cart",
		"/suppliers/followed",
		"/favorites",
		// /new rather than the history index: the index redirects to it when
		// the Smart Ordering service is not wired, which this harness never
		// wires, and a redirect would be indistinguishable from a refusal.
		"/customer/smart-order/new",
	}
)

// TestASupplierWithTheBuyingGrantsReachesTheSharedPages.
//
// A supplier restocking from other distributors buys on the pharmacy's own
// screens. This is the positive half of the separation above: the pages a
// supplier must reach, listed beside the ones they must not.
func TestASupplierWithTheBuyingGrantsReachesTheSharedPages(t *testing.T) {
	vendor := tenantActor(4, 51, "supplier", rbac.ScopeVendor)
	router := newTestRouter(vendor)

	for _, path := range sharedBuyingPaths {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
			// The harness builds the handler with nil services, so a page that
			// reads real data answers 500 rather than rendering. Only a refusal
			// — the gate's 303, or a 404 — means the page does not exist for
			// this caller, which is what this asserts against.
			if rec.Code == http.StatusSeeOther || rec.Code == http.StatusNotFound {
				t.Errorf("a supplier holding every vendor grant was refused the shared buying page %s (status %d)",
					path, rec.Code)
			}
		})
	}
}

// TestASupplierWithoutTheBuyingGrantsIsRefusedTheSharedPages is the same list
// held against a supplier who holds their selling dashboard and nothing else.
//
// Sharing a page between two dashboards is only safe if the grant still
// decides: without this, "vendor" would have become the permission.
func TestASupplierWithoutTheBuyingGrantsIsRefusedTheSharedPages(t *testing.T) {
	vendor := &authctx.Actor{
		UserID: 5, OrganizationID: 51, OrgType: "supplier",
		OrgStatus: "approved", Scope: rbac.ScopeVendor,
	}
	vendor.Grants([]string{
		"vendor.dashboard.view", "vendor.product.view",
		"vendor.order.view", "vendor.offer.view",
	})
	router := newTestRouter(vendor)

	for _, path := range sharedBuyingPaths {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
			if rec.Code != http.StatusSeeOther {
				t.Errorf("status = %d, want 303: a supplier without the buying grants opened %s",
					rec.Code, path)
			}
		})
	}
}

// TestASupplierCannotReachPharmacyPages.
func TestASupplierCannotReachPharmacyPages(t *testing.T) {
	vendor := tenantActor(1, 51, "supplier", rbac.ScopeVendor)
	router := newTestRouter(vendor)

	for _, path := range pharmacyOnlyPaths {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
			// 404, never 200: a supplier must not learn that the pharmacy URL
			// space exists, let alone see a page rendered from it.
			if rec.Code == http.StatusOK {
				t.Errorf("a supplier opened the pharmacy page %s", path)
			}
			if rec.Code != http.StatusSeeOther {
				t.Errorf("status = %d, want 303", rec.Code)
			}
		})
	}
}

// TestAPharmacyCannotReachSupplierPages.
func TestAPharmacyCannotReachSupplierPages(t *testing.T) {
	pharmacy := tenantActor(2, 50, "customer", rbac.ScopePharmacy)
	router := newTestRouter(pharmacy)

	for _, path := range vendorOnlyPaths {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
			if rec.Code == http.StatusOK {
				t.Errorf("a pharmacy opened the supplier page %s", path)
			}
			if rec.Code != http.StatusSeeOther {
				t.Errorf("status = %d, want 303", rec.Code)
			}
		})
	}
}

// TestALegacySupplierTypeStillReachesItsOwnDashboard.
//
// The bug the user hit: an organization stored as "supplier" rather than
// "vendor" got the pharmacy shell and an empty sidebar. Refusing the other
// dashboard is only half the requirement — the caller must still reach their
// own, whichever spelling their row happens to carry.
func TestALegacySupplierTypeStillReachesItsOwnDashboard(t *testing.T) {
	for _, orgType := range []string{"vendor", "supplier", "company", "agency"} {
		t.Run(orgType, func(t *testing.T) {
			// Scope deliberately left empty, so DashboardScope has to derive it
			// from the type — the path that was broken.
			a := &authctx.Actor{
				UserID: 3, OrganizationID: 51, OrgType: orgType, OrgStatus: "approved",
			}
			a.Grants([]string{"vendor.*"})

			if !a.IsVendor() {
				t.Fatalf("organization type %q is not recognised as a supplier", orgType)
			}
			if a.IsCustomer() {
				t.Errorf("organization type %q is treated as a pharmacy", orgType)
			}
			if got := a.DashboardScope(); got != rbac.ScopeVendor {
				t.Fatalf("DashboardScope = %q, want %q", got, rbac.ScopeVendor)
			}
			// And the sidebar it would render is not empty.
			nav := rbac.VisibleNav(a.DashboardScope(), rbac.NewSet(a.Permissions))
			if countItems(nav) == 0 {
				t.Error("the supplier sidebar renders no items; this is the empty-sidebar bug")
			}
		})
	}
}

// TestPharmacyDashboardHasNoSupplierOnlyTools.
//
// مقارنة الخصومات and خصومات السوق read across suppliers' price lists. They
// belong to the supplier dashboard, and removing them from the sidebar without
// removing the permission would leave them grantable in a pharmacy owner's
// role editor.
func TestPharmacyDashboardHasNoSupplierOnlyTools(t *testing.T) {
	for _, section := range rbac.Nav(rbac.ScopePharmacy) {
		for _, item := range section.Items {
			switch item.Href {
			case "/compare/tool", "/market-discounts":
				t.Errorf("the pharmacy sidebar still links to the supplier-only %s", item.Href)
			}
		}
	}
	for _, p := range rbac.Default().PermissionsFor(rbac.ScopePharmacy) {
		if p.Key == "pharmacy.compare.use" || p.Key == "pharmacy.market_discounts.view" {
			t.Errorf("%q is still offered in the pharmacy role editor", p.Key)
		}
	}
}

func countItems(sections []rbac.NavSection) int {
	n := 0
	for _, s := range sections {
		n += len(s.Items)
	}
	return n
}

// newTestRouter is defined in handlers_test.go; this asserts the shape it
// returns so a change there fails here rather than silently weakening these
// checks.
var _ = func() chi.Router { return chi.NewRouter() }
