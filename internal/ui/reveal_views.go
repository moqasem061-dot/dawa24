package ui

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/shared/mask"
)

// maskedOrganization returns a copy of the organization whose sensitive fields hold
// only their masked form (plan.md S8, FR-032). A page built from the copy cannot
// render a clear value, because the clear value is not in the view. The copy keeps
// the id, which the reveal control needs.
func maskedOrganization(o *org.Organization) *org.Organization {
	if o == nil {
		return nil
	}
	c := *o
	c.TaxNumber = mask.Number(o.TaxNumber)
	c.CommercialRegister = mask.Number(o.CommercialRegister)
	c.PharmacistLicense = mask.Number(o.PharmacistLicense)
	c.Phone = mask.Phone(o.Phone)
	c.Email = mask.Email(o.Email)
	return &c
}

// maskedShipment returns a copy of the shipment whose buyer branch phone is masked.
// The supplier's order page renders the copy; the reveal control names the order.
func maskedShipment(sh *commerce.OrderShipment) *commerce.OrderShipment {
	if sh == nil {
		return nil
	}
	c := *sh
	c.CustomerBranchPhone = mask.Phone(sh.CustomerBranchPhone)
	return &c
}
