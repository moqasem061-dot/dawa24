package ui

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The delivery branch, checked before a run exists.
//
// Its own file because smart_order_handlers.go is already over the 400-line
// limit AGENTS.md sets, and because this is one concern: everything a delivery
// branch must satisfy before it is worth matching a file against it.

// smartOrderBranchRefusal checks the delivery branch before a run is created,
// and returns the message to show when it cannot be used.
//
// Everything here is something commerce.CheckAvailability would refuse at
// checkout. Asking it now costs two reads and saves the buyer from mapping
// columns, waiting through a matching run and reviewing an order that was never
// placeable — which is precisely the "it only tells me at the last step"
// complaint, one step earlier than the Corporate Operations gate can reach.
//
// It is a UI-layer check because smartorder must not import org (AGENTS.md
// rule 5), and because the service's own validation cannot see branches.
func (h *UIHandler) smartOrderBranchRefusal(ctx context.Context, orgID, branchID int64, lang string) string {
	if branchID <= 0 {
		return i18n.T(lang, "smartorder.err_branch_required")
	}
	if h.orgSvc == nil {
		return ""
	}

	branch, err := h.orgSvc.GetBranch(database.AsSystem(ctx), branchID)
	if err != nil || branch == nil {
		return i18n.T(lang, "smartorder.err_branch_required")
	}
	// The dropdown only offers the buyer's own branches; a hand-written post is
	// not so constrained, and a run built against another company's branch
	// evaluates coverage and Corporate Operations against the wrong address.
	if branch.OrganizationID != orgID {
		return i18n.T(lang, "smartorder.err_branch_not_owned")
	}
	if (branch.Latitude == nil || branch.Longitude == nil) && (branch.CityID == nil || *branch.CityID <= 0) {
		return i18n.T(lang, "smartorder.err_branch_no_location")
	}

	hasWorks, err := h.orgSvc.BranchHasInstitutionalWorks(database.AsSystem(ctx), branchID)
	if err != nil {
		h.log.WarnContext(ctx, "could not read the delivery branch's institutional works",
			"branch_id", branchID, "error", err)
		return ""
	}
	if !hasWorks {
		return i18n.T(lang, "smartorder.err_branch_no_institutional_works")
	}
	return ""
}
