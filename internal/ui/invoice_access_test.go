package ui

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

func TestInvoiceAndOrderAccessIsLimitedToTheParties(t *testing.T) {
	buyerOrg, vendorOrg := int64(10), int64(20)
	order := &commerce.Order{CustomerID: 1, OrganizationID: &buyerOrg,
		Shipments: []*commerce.OrderShipment{{OrganizationID: vendorOrg}}}
	inv := &billing.Invoice{OrganizationID: vendorOrg, CustomerOrgID: &buyerOrg}

	for _, tc := range []struct {
		name string
		a    authctx.Actor
		want bool
	}{
		{"buyer org", authctx.Actor{UserID: 2, OrganizationID: buyerOrg}, true},
		{"fulfilling vendor", authctx.Actor{UserID: 3, OrganizationID: vendorOrg}, true},
		{"staff", authctx.Actor{UserID: 4, IsStaff: true}, true},
		{"other pharmacy", authctx.Actor{UserID: 5, OrganizationID: 99}, false},
		{"no org", authctx.Actor{UserID: 6}, false},
	} {
		if got := actorMayViewOrder(tc.a, order); got != tc.want {
			t.Errorf("%s: actorMayViewOrder = %v; want %v", tc.name, got, tc.want)
		}
		if got := actorMayViewInvoice(tc.a, inv); got != tc.want {
			t.Errorf("%s: actorMayViewInvoice = %v; want %v", tc.name, got, tc.want)
		}
	}
}
