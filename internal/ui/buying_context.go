package ui

import (
	"context"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The buying surface's view of the caller.
//
// These three questions come up on every purchasing screen and were answered
// three different ways before: "is this caller allowed to buy" was
// actor.IsCustomer(), which stopped being the right question when suppliers
// gained the purchasing section; "whose stock is this" was open-coded against
// actor.OrganizationID in some places and not asked at all in others; and
// "where do I send someone who may not be here" was a hardcoded
// /customer/dashboard even in handlers a supplier can reach.

// buyerOrgID returns the organization the caller is buying for, or 0 when the
// caller is not buying — a visitor, or platform staff.
//
// It is the company whose own stock must never be offered back to it. Every
// listing on the buying surface filters on this, and CheckAvailability refuses
// the same pairing, so a row that slips past a filter is still not orderable.
func buyerOrgID(ctx context.Context) int64 {
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsBuyer() {
		return 0
	}
	return actor.OrganizationID
}

// ownedByBuyer reports whether a supplier organization is the caller's own.
//
// A zero buyerOrgID (a visitor, or staff) owns nothing, so the listings stay
// complete for everyone who is not buying.
func ownedByBuyer(buyerOrg, supplierOrg int64) bool {
	return buyerOrg > 0 && supplierOrg == buyerOrg
}

// dashboardHome is where a caller belongs when they may not be where they are.
//
// The buying pages are shared, so "back to your dashboard" cannot be a
// constant: a supplier bounced off /cart must land on /vendor/dashboard, not
// on a pharmacy screen that answers them 404.
func dashboardHome(actor authctx.Actor) string {
	switch {
	case actor.IsStaff:
		return "/admin/dashboard"
	case actor.DashboardScope() == rbac.ScopeVendor:
		return "/vendor/dashboard"
	case actor.DashboardScope() == rbac.ScopePharmacy:
		return "/customer/dashboard"
	}
	return "/"
}

// redirectHome sends the caller to their own dashboard with a notice.
func (h *UIHandler) redirectHome(w http.ResponseWriter, r *http.Request, notice string) {
	actor, _ := authctx.From(r.Context())
	h.redirectWithNotice(w, r, dashboardHome(actor), "error", notice)
}
