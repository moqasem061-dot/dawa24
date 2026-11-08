package ui

import (
	"context"
	"errors"

	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// The permission keys a reveal may hold (plan.md S8, rbac catalogues).
const (
	revealPermStaff    = "platform.reveal.sensitive"
	revealPermSupplier = "vendor.reveal.customer_contact"
)

// errRevealNotFound means the record or the value is absent, or the caller may
// not see it. The handler answers REVEAL_NOT_FOUND for both, so a refusal never
// tells a caller whether a record exists.
var errRevealNotFound = errors.New("reveal: value not found")

// revealKind is one value a caller may reveal, one at a time. Permission is the
// key the caller must hold. Load reads the value through the service call the
// existing page already makes, so the reveal adds no query of its own. For the
// organization kinds the id is the organization; for branch.phone it is the
// order (the supplier's own shipment is found through its organization); for
// gateway.admin_username it is unused.
type revealKind struct {
	Permission string
	Load       func(ctx context.Context, h *UIHandler, actor authctx.Actor, id int64) (string, error)
}

// revealKinds is the allowlist. A kind missing from it, such as a gateway
// password, a virtual key, an AI provider key or an ERP key, answers
// REVEAL_NOT_REVEALABLE and is never read.
var revealKinds = map[string]revealKind{
	"org.tax_number":          {Permission: revealPermStaff, Load: revealOrgField(func(o *org.Organization) string { return o.TaxNumber })},
	"org.commercial_register": {Permission: revealPermStaff, Load: revealOrgField(func(o *org.Organization) string { return o.CommercialRegister })},
	"org.pharmacist_license":  {Permission: revealPermStaff, Load: revealOrgField(func(o *org.Organization) string { return o.PharmacistLicense })},
	"org.phone":               {Permission: revealPermStaff, Load: revealOrgField(func(o *org.Organization) string { return o.Phone })},
	"org.email":               {Permission: revealPermStaff, Load: revealOrgField(func(o *org.Organization) string { return o.Email })},
	"branch.phone":            {Permission: revealPermSupplier, Load: loadRevealBranchPhone},
	"gateway.admin_username":  {Permission: revealPermStaff, Load: loadRevealGatewayUsername},
}

// revealOrgField reads one field of an organization. plan.md E9: staff may reveal
// the five organization kinds. The read is the existing GetOrganization call the
// admin organization page makes under the system context, so no SQL is added here.
func revealOrgField(pick func(*org.Organization) string) func(context.Context, *UIHandler, authctx.Actor, int64) (string, error) {
	return func(ctx context.Context, h *UIHandler, _ authctx.Actor, id int64) (string, error) {
		if h.orgSvc == nil || id <= 0 {
			return "", errRevealNotFound
		}
		o, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), id)
		if err != nil || o == nil {
			return "", errRevealNotFound
		}
		value := pick(o)
		if value == "" {
			return "", errRevealNotFound
		}
		return value, nil
	}
}

// loadRevealBranchPhone reads the buyer branch phone of an order. The supplier
// sees it only for an order its own organization shipped: the lookup runs under
// the caller's organization, exactly as the vendor order page runs it, so an
// order the caller did not ship is not found.
func loadRevealBranchPhone(ctx context.Context, h *UIHandler, actor authctx.Actor, orderID int64) (string, error) {
	if h.commSvc == nil || actor.OrganizationID <= 0 || orderID <= 0 {
		return "", errRevealNotFound
	}
	sh, err := h.commSvc.GetVendorShipmentOrByOrderID(ctx, orderID, actor.OrganizationID)
	if err != nil || sh == nil || sh.CustomerBranchPhone == "" {
		return "", errRevealNotFound
	}
	return sh.CustomerBranchPhone, nil
}

// loadRevealGatewayUsername reads the stored gateway administrator username through
// the same settings read the admin developers page makes. The password and the
// key are never read into the reveal.
func loadRevealGatewayUsername(ctx context.Context, h *UIHandler, _ authctx.Actor, _ int64) (string, error) {
	if h.adminSvc == nil {
		return "", errRevealNotFound
	}
	gw, err := h.adminSvc.GetGatewaySettings(ctx)
	if err != nil || gw == nil {
		return "", errRevealNotFound
	}
	user, _ := gw.AdminCredentials()
	if user == "" {
		return "", errRevealNotFound
	}
	return user, nil
}
