package ui

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// accountManagedByOrg reports whether a company may add an existing account to
// its team or edit that account's profile: the account is not platform staff
// and belongs to no other organization.
//
// Team screens look accounts up by email and edit the global identity row
// (name, phone). Without this check any supplier could add a competitor's
// owner — or a platform administrator — to its own team by typing their email
// and then rewrite that person's name and phone number.
func (h *UIHandler) accountManagedByOrg(ctx context.Context, u *identity.User, orgID int64) bool {
	if u == nil || h.orgSvc == nil || identity.IsPrivilegedRole(u.Role) {
		return false
	}
	elsewhere, err := h.orgSvc.UserBelongsElsewhere(database.AsSystem(ctx), u.ID, orgID)
	return err == nil && !elsewhere
}
