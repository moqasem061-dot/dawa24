package authctx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func serve(gate func(http.Handler) http.Handler, actor *authctx.Actor, path string) int {
	req := httptest.NewRequest("GET", path, nil)
	if actor != nil {
		req = req.WithContext(authctx.WithActor(req.Context(), *actor))
	}
	rec := httptest.NewRecorder()
	gate(okHandler()).ServeHTTP(rec, req)
	return rec.Code
}

// TestTenantPageGateIsTheCompanyBoundary.
//
// Before this gate existed, /vendor/* and /customer/* were behind
// RequireVendor and RequireApproved only — "are you a member of an approved
// company of this type?". Any member of any such company could open any page
// in it by typing the URL, including the wallet, the team list and the role
// editor. These cases are that hole, closed.
func TestTenantPageGateIsTheCompanyBoundary(t *testing.T) {
	gate := authctx.RequireTenantPagePermission("vendor.wallet.view")

	member := func(perms ...string) *authctx.Actor {
		a := &authctx.Actor{UserID: 7, OrganizationID: 51, OrgType: "vendor", Scope: rbac.ScopeVendor}
		a.Grants(perms)
		return a
	}

	cases := []struct {
		name  string
		actor *authctx.Actor
		want  int
	}{
		{"anonymous is sent to sign in", nil, http.StatusSeeOther},
		{
			// The case the whole change exists for.
			name:  "a company member without the grant is refused",
			actor: member("vendor.dashboard.view", "vendor.order.view"),
			want:  http.StatusSeeOther,
		},
		{
			name:  "a company member with the grant passes",
			actor: member("vendor.wallet.view"),
			want:  http.StatusOK,
		},
		{
			name:  "a company owner holding the whole dashboard passes",
			actor: member("vendor.*"),
			want:  http.StatusOK,
		},
		{
			name: "a member with no company is refused",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{UserID: 8, OrgType: "vendor"}
				a.Grants([]string{"vendor.*"})
				return a
			}(),
			want: http.StatusSeeOther,
		},
		{
			// Staff authority over a tenant comes from platform permissions,
			// not from walking into the tenant's own dashboard.
			name: "platform staff do not reach a company dashboard",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{UserID: 1, IsStaff: true, OrganizationID: 51, Role: "super_admin"}
				a.Grants([]string{"*"})
				return a
			}(),
			want: http.StatusSeeOther,
		},
		{
			// A grant for the other tenant dashboard is not this one.
			name:  "a pharmacy grant does not open a vendor page",
			actor: member("pharmacy.wallet.view"),
			want:  http.StatusSeeOther,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serve(gate, tc.actor, "/vendor/wallet"); got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestAdminGateHasNoRoleNameBypass.
//
// The gate used to pass anyone whose role string was "super_admin" or
// "developer", regardless of grants. That made two role names load-bearing:
// a new staff role could not be given full access, and the developer role
// silently held every admin page whatever its permissions said.
func TestAdminGateHasNoRoleNameBypass(t *testing.T) {
	gate := authctx.RequirePagePermission("platform.developer.sql")

	staff := func(role string, perms ...string) *authctx.Actor {
		a := &authctx.Actor{UserID: 3, IsStaff: true, Role: role, Scope: rbac.ScopeAdmin}
		a.Grants(perms)
		return a
	}

	if got := serve(gate, staff("developer"), "/admin/developers"); got != http.StatusSeeOther {
		t.Errorf("a role named 'developer' with no grants passed the gate: status %d", got)
	}
	if got := serve(gate, staff("super_admin"), "/admin/developers"); got != http.StatusSeeOther {
		t.Errorf("a role named 'super_admin' with no grants passed the gate: status %d", got)
	}
	if got := serve(gate, staff("super_admin", "*"), "/admin/developers"); got != http.StatusOK {
		t.Errorf("the owner grant did not open the page: status %d", got)
	}
	// A role nobody wrote into this file works, which is the point.
	if got := serve(gate, staff("finance_moderator", "platform.developer.sql"), "/admin/developers"); got != http.StatusOK {
		t.Errorf("a custom staff role with the grant was refused: status %d", got)
	}
	// A non-staff account never reaches /admin, grant or no grant.
	notStaff := &authctx.Actor{UserID: 9, Role: "customer"}
	notStaff.Grants([]string{"*"})
	if got := serve(gate, notStaff, "/admin/developers"); got != http.StatusSeeOther {
		t.Errorf("a non-staff account with a wildcard grant reached /admin: status %d", got)
	}
}

// TestAnyOfGates covers a page two roles reach for different reasons.
func TestAnyOfGates(t *testing.T) {
	gate := authctx.RequirePagePermission("billing.invoice.view", "billing.payment.view")
	staff := func(perms ...string) *authctx.Actor {
		a := &authctx.Actor{UserID: 4, IsStaff: true, Role: "support"}
		a.Grants(perms)
		return a
	}
	if got := serve(gate, staff("billing.payment.view"), "/admin/finance"); got != http.StatusOK {
		t.Errorf("holding the second alternative was refused: status %d", got)
	}
	if got := serve(gate, staff("commerce.order.view"), "/admin/finance"); got != http.StatusSeeOther {
		t.Errorf("holding neither alternative passed: status %d", got)
	}
}

// TestActorCanUsesHierarchicalMatching. Actor.Can compared strings exactly,
// while RequirePagePermission separately special-cased the "*" grant — so the
// two answered the same question differently.
func TestActorCanUsesHierarchicalMatching(t *testing.T) {
	a := &authctx.Actor{UserID: 1}
	a.Grants([]string{"vendor.*"})

	if !a.Can("vendor.order.view") {
		t.Error("a module-wide grant did not satisfy a permission inside it")
	}
	if a.Can("pharmacy.order.view") {
		t.Error("a vendor grant satisfied a pharmacy permission")
	}
	if !a.CanAny("platform.setting.view", "vendor.order.view") {
		t.Error("CanAny missed a held alternative")
	}
	if a.CanAll("vendor.order.view", "platform.setting.view") {
		t.Error("CanAll passed while one permission was not held")
	}

	// An actor built without Grants still answers from the Permissions field,
	// because handlers and tests populate it directly.
	direct := authctx.Actor{UserID: 2, Permissions: []string{"vendor.order.view"}}
	if !direct.Can("vendor.order.view") {
		t.Error("a directly populated Permissions slice was ignored")
	}
}
