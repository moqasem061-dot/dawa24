package ui

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// canSeeContactDetails reports whether the viewer may see a supplier's
// registration numbers and branch contacts: staff, or a signed-in user whose own
// organization is approved (Batch 3, S14).
func canSeeContactDetails(actor authctx.Actor) bool {
	return actor.IsStaff || actor.OrgStatus == string(org.StatusApproved)
}

// hideContactDetails blanks the tax number, commercial register and each
// branch's phone and manager name. The branches are copied, so the caller's
// slice is not changed in place.
func hideContactDetails(o *org.Organization, branches []*org.Branch) []*org.Branch {
	o.TaxNumber = ""
	o.CommercialRegister = ""
	hidden := make([]*org.Branch, 0, len(branches))
	for _, b := range branches {
		if b == nil {
			continue
		}
		c := *b
		c.Phone = ""
		c.ManagerName = ""
		hidden = append(hidden, &c)
	}
	return hidden
}
