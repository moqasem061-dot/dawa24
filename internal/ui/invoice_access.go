package ui

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// Invoice print and export pages load their record by the id in the URL.
// Orders are read as system, so without these checks any member holding the
// invoice-view grant could print another company's order — buyer, address,
// lines and prices — by counting ids, and the print page would even file an
// invoice for it.

// actorMayViewOrder reports whether a is staff, the buyer of o, or a vendor
// fulfilling part of it.
func actorMayViewOrder(a authctx.Actor, o *commerce.Order) bool {
	if o == nil {
		return false
	}
	if a.IsStaff {
		return true
	}
	if a.UserID > 0 && o.CustomerID == a.UserID {
		return true
	}
	org := a.OrganizationID
	if org <= 0 {
		return false
	}
	if o.OrganizationID != nil && *o.OrganizationID == org {
		return true
	}
	for _, s := range o.Shipments {
		if s != nil && s.OrganizationID == org {
			return true
		}
	}
	for _, l := range o.Lines {
		if l != nil && l.OrganizationID == org {
			return true
		}
	}
	return false
}

// actorMayViewInvoice reports whether a is staff, the issuer or the billed
// customer of inv.
func actorMayViewInvoice(a authctx.Actor, inv *billing.Invoice) bool {
	if inv == nil {
		return false
	}
	if a.IsStaff {
		return true
	}
	org := a.OrganizationID
	return org > 0 && (inv.OrganizationID == org || (inv.CustomerOrgID != nil && *inv.CustomerOrgID == org))
}
