package ui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Sidebar and route, held against each other on the real router.
//
// The requirement is not "hide the link" and it is not "gate the route" — it is
// that the two agree, for every item, in every dashboard. A hidden link over an
// open page is a page anyone can type their way into; a visible link over a
// closed page is a door that will not open.
//
// This used to be checked textually, by grepping the route files for the
// permission strings the sidebar names. That check could only see gates written
// as string literals, so it went blind the moment a gate named a
// rbac.Capability instead — and it could never have caught a gate that names
// the right key on the wrong path.
//
// So it is behavioural now. For every item in every dashboard it builds two
// callers, holding everything in the scope with and without that item's own
// permissions, and issues a real request for the item's href through the router
// cmd/server builds.

// sidebarExceptions are items whose destination is deliberately not gated on
// the permission that reveals them. Each is a decision; say why.
var sidebarExceptions = map[string]string{
	// Public marketplace pages. A signed-out visitor may browse them, so they
	// carry no permission gate and the sidebar's copy of the link cannot hide
	// them. The sidebar points at the gated /customer/* copies wherever one
	// exists; these two have none.
	"vendor compare":          "/compare/tool is a public page, reachable signed out",
	"vendor market-discounts": "/market-discounts is a public page, reachable signed out",

	// The staff landing page: RequireStaff is its gate, and a staff member
	// holding nothing else still has to land somewhere.
	"admin dashboard": "every staff member reaches /admin/dashboard",

	// Pre-approval destinations. A company under review holds no dashboard
	// permission at all and must still be able to send its papers and read
	// what the platform is telling it.
	"vendor documents":       "the documents page is in the pre-approval tier",
	"pharmacy documents":     "the documents page is in the pre-approval tier",
	"vendor notifications":   "the notifications centre is in the pre-approval tier",
	"pharmacy notifications": "the notifications centre is in the pre-approval tier",
}

// refusedByGate reports whether the router turned this caller away from href.
//
// A bare 303 is not enough to tell with: the harness builds the handler with nil
// services, so several pages redirect to their dashboard with "service
// unavailable" and look exactly like a refusal. Every gate in authctx refuses
// through redirectUnauthorized, which stamps auth_notice=forbidden on the
// destination — that, and a 404, are the two things a refusal looks like.
func refusedByGate(router http.Handler, href string) (refused bool) {
	defer func() {
		// A panic means the handler ran, which means the gate let the caller
		// through. That is the answer this function exists to give.
		_ = recover()
	}()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, href, nil))
	if rec.Code == http.StatusNotFound {
		return true
	}
	return strings.Contains(rec.Header().Get("Location"), "auth_notice=forbidden")
}

// scopeActor builds a member of a company on one dashboard, holding the keys
// given.
func scopeActor(scope rbac.Scope, keys []string) *authctx.Actor {
	orgType := "customer"
	orgID := int64(700)
	if scope == rbac.ScopeVendor {
		orgType = "vendor"
		orgID = 701
	}
	a := &authctx.Actor{
		UserID: 70, OrganizationID: orgID, OrgType: orgType,
		OrgStatus: "approved", Scope: scope,
	}
	if scope == rbac.ScopeAdmin {
		a = &authctx.Actor{UserID: 70, IsStaff: true, Role: "support", Scope: scope}
	}
	a.Grants(keys)
	return a
}

// keysExcept is every permission the scope declares except the ones given.
func keysExcept(scope rbac.Scope, drop ...string) []string {
	dropped := make(map[string]bool, len(drop))
	for _, d := range drop {
		if d != "" {
			dropped[d] = true
		}
	}
	all := rbac.Default().KeysFor(scope)
	out := make([]string, 0, len(all))
	for _, k := range all {
		if !dropped[k] {
			out = append(out, k)
		}
	}
	return out
}

// TestEverySidebarItemIsRevealedAndGatedByTheSamePermission.
func TestEverySidebarItemIsRevealedAndGatedByTheSamePermission(t *testing.T) {
	checked := 0
	for _, scope := range rbac.Scopes() {
		for _, section := range rbac.Nav(scope) {
			for _, item := range section.Items {
				name := string(scope) + " " + item.Key
				if _, skip := sidebarExceptions[name]; skip {
					continue
				}
				t.Run(name, func(t *testing.T) {
					checked++
					perms := append([]string{item.Perm}, item.Also...)

					holder := scopeActor(scope, rbac.Default().KeysFor(scope))
					if !item.Visible(rbac.NewSet(holder.Permissions)) {
						t.Fatalf("the item is hidden from a caller holding the whole %s dashboard", scope)
					}
					if refusedByGate(newTestRouter(holder), item.Href) {
						t.Errorf("%s is refused to a caller holding %v", item.Href, perms)
					}

					if item.AlwaysVisible {
						// An item about the caller rather than the company has
						// no permission to withhold; the floor is that it stays
						// reachable however little they hold.
						empty := scopeActor(scope, nil)
						if !item.Visible(rbac.NewSet(nil)) {
							t.Error("an always-visible item is hidden from a caller holding nothing")
						}
						if refusedByGate(newTestRouter(empty), item.Href) {
							t.Errorf("%s is refused to a caller holding nothing, but it is always-visible", item.Href)
						}
						return
					}

					withoutIt := scopeActor(scope, keysExcept(scope, perms...))
					if item.Visible(rbac.NewSet(withoutIt.Permissions)) {
						t.Errorf("the item is still shown to a caller holding everything except %v", perms)
					}
					if !refusedByGate(newTestRouter(withoutIt), item.Href) {
						t.Errorf("%s is reachable by a caller holding everything except %v; "+
							"the link is hidden but the page is open to anyone who types the URL",
							item.Href, perms)
					}
				})
			}
		}
	}
	if checked == 0 {
		t.Fatal("no sidebar items were checked; the registry shape changed")
	}
}

// TestSidebarExceptionsStillNameRealItems keeps the exception list from
// outliving what it excuses. An entry naming an item that no longer exists is
// an exemption nobody is watching.
func TestSidebarExceptionsStillNameRealItems(t *testing.T) {
	live := map[string]bool{}
	for _, scope := range rbac.Scopes() {
		for _, section := range rbac.Nav(scope) {
			for _, item := range section.Items {
				live[string(scope)+" "+item.Key] = true
			}
		}
	}
	for name, why := range sidebarExceptions {
		if !live[name] {
			t.Errorf("sidebarExceptions still excuses %q (%s), which is no longer a sidebar item", name, why)
		}
	}
}
