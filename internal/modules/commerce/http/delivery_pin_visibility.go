package http

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// The buyer's delivery PIN is the buyer's to give to the person who brings the
// parcel. A supplier that could read it could close its own parcel without the
// buyer, so the API shows it only to the pharmacy that placed the order and to
// staff. The PIN stays in the database and on the buyer's screens.

// pinVisibleTo reports whether the actor may read the delivery PIN of the order.
func pinVisibleTo(actor authctx.Actor, order *commerce.Order) bool {
	if actor.IsStaff || actor.Can("commerce.admin") {
		return true
	}
	return order != nil && (order.CustomerID == actor.UserID ||
		(order.OrganizationID != nil && *order.OrganizationID == actor.OrganizationID))
}

// hideOrderPINs blanks the delivery PIN of every shipment of an order the actor
// may not see the PIN for.
func hideOrderPINs(actor authctx.Actor, order *commerce.Order) {
	if order == nil || pinVisibleTo(actor, order) {
		return
	}
	for _, s := range order.Shipments {
		if s != nil {
			s.DeliveryCode = ""
		}
	}
}

// hideShipmentPINs blanks the delivery PIN of parcels a supplier reads on its own
// list. A supplier is never the buyer, so the PIN is hidden unless the actor is staff.
func hideShipmentPINs(actor authctx.Actor, shipments []*commerce.OrderShipment) {
	if actor.IsStaff || actor.Can("commerce.admin") {
		return
	}
	for _, s := range shipments {
		if s != nil {
			s.DeliveryCode = ""
		}
	}
}
