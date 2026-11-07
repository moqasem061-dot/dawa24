package http

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// The buyer's delivery PIN is shown to the pharmacy that placed the order and to
// staff. A supplier reading the order or its parcels must not receive it.
func TestDeliveryPINVisibility(t *testing.T) {
	const buyerUser, buyerOrg, vendorUser, vendorOrg = 1, 100, 2, 200
	buyerOrgID := int64(buyerOrg)
	order := func() *commerce.Order {
		return &commerce.Order{
			ID: 9, CustomerID: buyerUser, OrganizationID: &buyerOrgID,
			Shipments: []*commerce.OrderShipment{{ID: 5, OrganizationID: vendorOrg, DeliveryCode: "482913"}},
		}
	}
	cases := []struct {
		name    string
		actor   authctx.Actor
		visible bool
	}{
		{name: "buyer user", actor: authctx.Actor{UserID: buyerUser, OrganizationID: 0}, visible: true},
		{name: "buyer organization member", actor: authctx.Actor{UserID: 7, OrganizationID: buyerOrg}, visible: true},
		{name: "supplier member", actor: authctx.Actor{UserID: vendorUser, OrganizationID: vendorOrg}},
		{name: "unrelated user", actor: authctx.Actor{UserID: 8, OrganizationID: 300}},
		{name: "staff", actor: authctx.Actor{UserID: 9, IsStaff: true}, visible: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pinVisibleTo(tc.actor, order()); got != tc.visible {
				t.Fatalf("pinVisibleTo = %v, want %v", got, tc.visible)
			}
			o := order()
			hideOrderPINs(tc.actor, o)
			want := "482913"
			if !tc.visible {
				want = ""
			}
			if got := o.Shipments[0].DeliveryCode; got != want {
				t.Fatalf("order shipment PIN = %q, want %q", got, want)
			}
		})
	}

	supplier := authctx.Actor{UserID: vendorUser, OrganizationID: vendorOrg}
	list := []*commerce.OrderShipment{{ID: 5, DeliveryCode: "482913"}}
	hideShipmentPINs(supplier, list)
	if list[0].DeliveryCode != "" {
		t.Fatalf("supplier list still shows the PIN")
	}
}
