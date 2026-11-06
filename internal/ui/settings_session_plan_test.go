package ui

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

// S22 (owner decision, 2026-11-06): a session plan paid from a company wallet needs the same
// subscription permission that the wallet subscription checkout requires.
func TestSessionPlanPayerAllowed(t *testing.T) {
	individual := authctx.Actor{UserID: 7}

	pharmacyManager := authctx.Actor{UserID: 7, OrganizationID: 3}
	pharmacyManager.Grants([]string{"pharmacy.subscription.manage"})

	pharmacyEmployee := authctx.Actor{UserID: 8, OrganizationID: 3}
	pharmacyEmployee.Grants([]string{"pharmacy.wallet.view"})

	vendorManager := authctx.Actor{UserID: 7, OrganizationID: 4, Scope: rbac.ScopeVendor}
	vendorManager.Grants([]string{"vendor.subscription.manage"})

	// A pharmacy permission does not open a vendor's company wallet.
	vendorEmployee := authctx.Actor{UserID: 8, OrganizationID: 4, Scope: rbac.ScopeVendor}
	vendorEmployee.Grants([]string{"pharmacy.subscription.manage"})

	cases := []struct {
		name  string
		actor authctx.Actor
		payer int64
		want  bool
	}{
		{name: "buyer pays own wallet", actor: individual, payer: 7, want: true},
		{name: "no payer resolved", actor: individual, payer: 0, want: true},
		{name: "pharmacy employee pays owner wallet without permission", actor: pharmacyEmployee, payer: 9, want: false},
		{name: "pharmacy manager pays owner wallet", actor: pharmacyManager, payer: 9, want: true},
		{name: "vendor employee pays owner wallet with a pharmacy permission", actor: vendorEmployee, payer: 9, want: false},
		{name: "vendor manager pays owner wallet", actor: vendorManager, payer: 9, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionPlanPayerAllowed(tc.actor, tc.payer); got != tc.want {
				t.Fatalf("sessionPlanPayerAllowed = %v, want %v", got, tc.want)
			}
		})
	}
}
