package test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// These tests walk the source the way admin_guard_test.go does. They exist
// because the failure they prevent is invisible: an ungated route behaves
// exactly like a gated one until somebody who should not reach it does, and a
// gate naming a permission nobody can hold behaves exactly like a missing
// feature. Neither shows up in manual testing by an account that holds
// everything — which is how the whole vendor and pharmacy surface came to be
// ungated without anyone noticing.

var (
	reGroup      = regexp.MustCompile(`(?s)r\.Group\(func\(g chi\.Router\) \{(.*?)\n\t\}\)`)
	reGateKeys   = regexp.MustCompile(`Require(?:Tenant)?PagePermission\(([^)]*)\)`)
	reQuoted     = regexp.MustCompile(`"([^"]+)"`)
	reRouteInSrc = regexp.MustCompile(`\bg\.(Get|Post|Put|Delete|Patch)\("([^"]+)"`)
	reBareRoute  = regexp.MustCompile(`\br\.(Get|Post|Put|Delete|Patch)\("(/(?:admin|vendor|customer)/[^"]*)"`)
)

// routeFiles are the registrars whose routes must all be gated on a permission.
var routeFiles = []string{
	"internal/ui/admin_routes_catalog.go",
	"internal/ui/admin_routes_commerce.go",
	"internal/ui/admin_routes_identity.go",
	"internal/ui/admin_routes_org.go",
	"internal/ui/admin_routes_pagecontrol.go",
	"internal/ui/admin_routes_platform.go",
	"internal/ui/vendor_routes.go",
	"internal/ui/vendor_catalog_routes.go",
	"internal/ui/customer_routes.go",
	"internal/ui/buying_routes.go",
}

// TestEveryRouteGateNamesADeclaredPermission walks every permission gate in the
// entire codebase (RequireAPIPermission, RequireAPITenantPermission,
// RequirePagePermission, RequireTenantPagePermission, RequirePermission) and asserts
// that each referenced key is declared in internal/platform/rbac.
func TestEveryRouteGateNamesADeclaredPermission(t *testing.T) {
	const root = ".."
	catalog := rbac.Default()
	reAnyGate := regexp.MustCompile(`Require(?:API|Tenant)?(?:Page)?Permission\(([^)]*)\)`)

	foundCallSites := 0
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_templ.go") {
				return nil
			}
			// Skip definition in authctx/audience.go and authctx/middleware.go
			rel, _ := filepath.Rel(root, path)
			relSlash := filepath.ToSlash(rel)
			if relSlash == "internal/platform/authctx/audience.go" || relSlash == "internal/platform/authctx/middleware.go" {
				return nil
			}

			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			for _, m := range reAnyGate.FindAllStringSubmatch(string(src), -1) {
				foundCallSites++
				for _, k := range reQuoted.FindAllStringSubmatch(m[1], -1) {
					key := k[1]
					if !catalog.Known(key) {
						t.Errorf("%s gates on undeclared permission key %q; no role can hold it", relSlash, key)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", dir, err)
		}
	}

	if foundCallSites == 0 {
		t.Fatalf("no permission gate call sites found; regex or directory walk is broken")
	}
}

// TestTenantGatesUseTenantScopedPermissions asserts that every gate call site in the
// codebase enforces keys matching its audience scope:
//   - Admin gates (RequirePagePermission, RequireAPIPermission, RequirePermission) must only use ScopeAdmin keys.
//   - Tenant gates (RequireTenantPagePermission, RequireAPITenantPermission) must never use ScopeAdmin keys,
//     and must match ScopeVendor for vendor routes / ScopePharmacy for pharmacy/customer routes.
func TestTenantGatesUseTenantScopedPermissions(t *testing.T) {
	const root = ".."
	catalog := rbac.Default()

	// RequirePagePermission and RequireAPIPermission both refuse a non-staff
	// caller outright, so every key they name has to be grantable on the admin
	// dashboard.
	//
	// authctx.RequirePermission is deliberately excluded. It is the third
	// category this audit used to be missing: a gate that asks only "does the
	// caller hold one of these", for a route both a staff member and a company
	// member legitimately reach — settling an invoice, topping up a wallet.
	// Naming a tenant key there is the point of it, and treating it as an
	// admin gate reported 27 correct call sites as violations, which is how an
	// audit stops being read. Its keys are still held to the catalogue by
	// TestEveryRouteGateNamesADeclaredPermission.
	reAdminGate := regexp.MustCompile(`Require(?:Page|API)Permission\(([^)]*)\)`)
	reTenantGate := regexp.MustCompile(`Require(?:API)?Tenant(?:Page)?Permission\(([^)]*)\)`)

	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_templ.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			relSlash := filepath.ToSlash(rel)
			if relSlash == "internal/platform/authctx/audience.go" || relSlash == "internal/platform/authctx/middleware.go" || relSlash == "internal/platform/authctx/testhelper.go" {
				return nil
			}

			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(src)

			// 1. Admin gates must use ScopeAdmin keys
			for _, m := range reAdminGate.FindAllStringSubmatch(content, -1) {
				for _, k := range reQuoted.FindAllStringSubmatch(m[1], -1) {
					key := k[1]
					p, ok := catalog.Lookup(key)
					if !ok {
						continue // covered by TestEveryRouteGateNamesADeclaredPermission
					}
					if !p.InScope(rbac.ScopeAdmin) {
						t.Errorf("%s gates an admin route on %q, which is not in ScopeAdmin", relSlash, key)
					}
				}
			}

			// 2. Tenant gates must use tenant keys (ScopeVendor or ScopePharmacy, never ScopeAdmin)
			for _, m := range reTenantGate.FindAllStringSubmatch(content, -1) {
				for _, k := range reQuoted.FindAllStringSubmatch(m[1], -1) {
					key := k[1]
					p, ok := catalog.Lookup(key)
					if !ok {
						continue // covered by TestEveryRouteGateNamesADeclaredPermission
					}
					if p.InScope(rbac.ScopeAdmin) && !p.InScope(rbac.ScopeVendor) && !p.InScope(rbac.ScopePharmacy) {
						t.Errorf("%s gates a tenant route on %q, which is an admin-only key", relSlash, key)
					}
					if strings.Contains(relSlash, "vendor") && !p.InScope(rbac.ScopeVendor) {
						t.Errorf("%s gates a vendor route on %q, which is not grantable in ScopeVendor", relSlash, key)
					}
					if (strings.Contains(relSlash, "customer") || strings.Contains(relSlash, "pharmacy")) && !p.InScope(rbac.ScopePharmacy) {
						t.Errorf("%s gates a customer route on %q, which is not grantable in ScopePharmacy", relSlash, key)
					}
				}
			}

			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", dir, err)
		}
	}
}

// TestNoDashboardRouteIsRegisteredOutsideAGate.
//
// Every route in these files must sit inside an r.Group whose Use(...) applies
// a permission gate. A route registered on the bare router is reachable by any
// member of the audience — which is what the entire vendor and pharmacy
// surface used to be.
func TestNoDashboardRouteIsRegisteredOutsideAGate(t *testing.T) {
	const root = ".."

	// Routes that are deliberately ungated, with the reason. Anything else
	// registered outside a group fails this test.
	allowed := map[string]string{
		"/vendor/password":          "changing your own password is an account action, not a company one",
		"/customer/password":        "changing your own password is an account action, not a company one",
		"/customer/set-branch":      "choosing which of your own branches you buy for is not a privilege",
		"/customer/branches/active": "choosing which of your own branches you buy for is not a privilege",
		"/customer/session":         "singular alias redirecting 301 to /customer/sessions",
		"/vendor/session":           "singular alias redirecting 301 to /vendor/sessions",
		"/customer/notifications":   "canonical 301 alias to /notifications",
		"/vendor/notifications":     "canonical 301 alias to /notifications",
		"/admin/dashboard":          "reachable by any authenticated staff member; RequireStaff is the gate",
	}

	for _, rel := range routeFiles {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		body := string(src)

		// Remove every gated group, then look at what is left.
		outside := reGroup.ReplaceAllString(body, "")
		for _, m := range reBareRoute.FindAllStringSubmatch(outside, -1) {
			if _, ok := allowed[m[2]]; ok {
				continue
			}
			t.Errorf("%s registers %s %s outside any permission group; "+
				"wrap it in r.Group(func(g chi.Router){ g.Use(authctx.Require…PagePermission(\"…\")) … }) "+
				"or add it to the allowed list above with a reason",
				rel, m[1], m[2])
		}

		// And every group that does contain routes must apply a gate.
		for _, g := range reGroup.FindAllStringSubmatch(body, -1) {
			if !reRouteInSrc.MatchString(g[1]) {
				continue
			}
			if !strings.Contains(g[1], "PagePermission(") && !strings.Contains(g[1], "RequireCapability(") {
				routes := reRouteInSrc.FindAllStringSubmatch(g[1], -1)
				paths := make([]string, 0, len(routes))
				for _, r := range routes {
					paths = append(paths, r[2])
				}
				t.Errorf("%s has a route group with no permission gate, covering: %s",
					rel, strings.Join(paths, ", "))
			}
		}
	}
}

// The sidebar-to-route direction used to be checked here, textually: every
// permission a sidebar item names had to appear inside some gate in the route
// files. It could only see gates written as string literals, so it went blind
// the moment a shared page was gated on a rbac.Capability, and it could never
// have caught a gate naming the right key on the wrong path.
//
// internal/ui/sidebar_route_sync_test.go replaces it with the behavioural
// check: every item, in every dashboard, requested through the real router by a
// caller who holds its permission and by one who holds everything else.
