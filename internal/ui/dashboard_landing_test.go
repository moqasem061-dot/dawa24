package ui

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Where a member of a company lands after signing in.
//
// This used to be one hardcoded URL per organisation type, which was true only
// while every company role held the dashboard. A مندوب holds إدارة الشحنات and
// nothing else, so that assumption turned their sign-in into a 404.
func TestDashboardLanding(t *testing.T) {
	courier, _ := rbac.OrganizationRole("org_courier")
	manager, _ := rbac.OrganizationRole("org_manager")

	cases := []struct {
		name     string
		scope    rbac.Scope
		perms    []string
		fallback string
		want     string
	}{
		{
			name:     "a delivery representative lands on their own portal",
			scope:    rbac.ScopeVendor,
			perms:    rbac.GrantsFor(courier, rbac.ScopeVendor),
			fallback: "/vendor/dashboard",
			want:     "/vendor/delivery",
		},
		{
			name:     "an ordinary supplier member still lands on the dashboard",
			scope:    rbac.ScopeVendor,
			perms:    rbac.GrantsFor(manager, rbac.ScopeVendor),
			fallback: "/vendor/dashboard",
			want:     "/vendor/dashboard",
		},
		{
			name:     "a pharmacy member lands on the pharmacy dashboard",
			scope:    rbac.ScopePharmacy,
			perms:    rbac.GrantsFor(manager, rbac.ScopePharmacy),
			fallback: "/customer/dashboard",
			want:     "/customer/dashboard",
		},
		{
			// A member with nothing granted has no first screen. The fallback
			// is the honest answer: the dashboard will refuse them, and being
			// refused by the page they expected beats being redirected into a
			// page they did not ask for.
			name:     "a member holding nothing falls back",
			scope:    rbac.ScopeVendor,
			perms:    nil,
			fallback: "/vendor/dashboard",
			want:     "/vendor/dashboard",
		},
		{
			// Account settings is about the caller, not the company, so it is
			// never chosen as a landing page even though everyone sees it.
			name:     "account settings is not a landing page",
			scope:    rbac.ScopeVendor,
			perms:    []string{"vendor.session.view"},
			fallback: "/vendor/dashboard",
			want:     "/vendor/sessions",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dashboardLanding(tc.scope, tc.perms, tc.fallback); got != tc.want {
				t.Errorf("dashboardLanding = %q, want %q", got, tc.want)
			}
		})
	}
}
