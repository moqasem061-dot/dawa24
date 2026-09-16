package ui

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Whose stock is this, and is it the caller's own?
//
// Every listing on the buying surface asks these two questions, and the answer
// has to be the same everywhere: the catalogue, the offers board, the supplier
// directory, the followed list and the cart all read buyerOrgID and compare
// through ownedByBuyer. commerce.CheckAvailability refuses the same pairing, so
// a row that slipped past a filter would still not be orderable — but it would
// be visible, and the requirement is that it never is.

func buyerCtx(scope rbac.Scope, orgID int64) context.Context {
	orgType := "customer"
	if scope == rbac.ScopeVendor {
		orgType = "vendor"
	}
	return authctx.WithActor(context.Background(), authctx.Actor{
		UserID: 1, OrganizationID: orgID, OrgType: orgType,
		OrgStatus: "approved", Scope: scope,
	})
}

func TestBuyerOrgID(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want int64
	}{
		{
			name: "a pharmacy member buys for their pharmacy",
			ctx:  buyerCtx(rbac.ScopePharmacy, 50),
			want: 50,
		},
		{
			// The case this whole change exists for.
			name: "a supplier member buys for their own company",
			ctx:  buyerCtx(rbac.ScopeVendor, 51),
			want: 51,
		},
		{
			name: "a visitor buys for nobody",
			ctx:  context.Background(),
			want: 0,
		},
		{
			// Staff have no company and no cart; nothing in a listing is
			// "theirs", so nothing is hidden from them.
			name: "platform staff buy for nobody",
			ctx: authctx.WithActor(context.Background(), authctx.Actor{
				UserID: 2, IsStaff: true, Role: "admin",
			}),
			want: 0,
		},
		{
			name: "a member with no organization buys for nobody",
			ctx: authctx.WithActor(context.Background(), authctx.Actor{
				UserID: 3, OrgType: "customer", OrgStatus: "approved",
			}),
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buyerOrgID(tc.ctx); got != tc.want {
				t.Errorf("buyerOrgID = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestOwnedByBuyer(t *testing.T) {
	cases := []struct {
		name            string
		buyer, supplier int64
		want            bool
	}{
		{"a supplier's own stock", 51, 51, true},
		{"another supplier's stock", 51, 52, false},
		{"a visitor owns nothing", 0, 51, false},
		// A supplier id of zero is a malformed row, not a match with a
		// caller who has no company: a filter that treated it as one would
		// hide every unattributed line from every visitor.
		{"an unattributed line is not the visitor's", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ownedByBuyer(tc.buyer, tc.supplier); got != tc.want {
				t.Errorf("ownedByBuyer(%d, %d) = %v, want %v", tc.buyer, tc.supplier, got, tc.want)
			}
		})
	}
}

func TestDashboardHome(t *testing.T) {
	cases := []struct {
		name  string
		actor authctx.Actor
		want  string
	}{
		{"a supplier", authctx.Actor{UserID: 1, OrganizationID: 51, OrgType: "vendor", Scope: rbac.ScopeVendor}, "/vendor/dashboard"},
		{"a pharmacy", authctx.Actor{UserID: 1, OrganizationID: 50, OrgType: "customer", Scope: rbac.ScopePharmacy}, "/customer/dashboard"},
		{"platform staff", authctx.Actor{UserID: 2, IsStaff: true}, "/admin/dashboard"},
		{"nobody", authctx.Actor{}, "/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dashboardHome(tc.actor); got != tc.want {
				t.Errorf("dashboardHome = %q, want %q", got, tc.want)
			}
		})
	}
}
