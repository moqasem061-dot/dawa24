package authctx

import "github.com/muhiya/dawa24-store/internal/platform/rbac"

// ApplyGrant overlays a freshly resolved grant onto an actor.
//
// It is the one place a caller's live authority is turned into an Actor. The
// browser path starts from the session and applies the grant on top; a path
// with no session — a Telegram message — starts from nothing but the user and
// organisation ids and applies the same grant. Two interfaces, one answer to
// "who is this and what may they do".
//
// Empty grant fields leave the actor's value alone, so a session's copy
// survives where the database had nothing newer to say.
func ApplyGrant(a *Actor, g rbac.Grant) {
	a.IsStaff = g.IsStaff
	a.IsOwner = g.IsPlatformOwner || g.IsOrgOwner
	a.Scope = g.Scope
	if g.PlatformRole != "" {
		a.Role = g.PlatformRole
	}
	if g.OrgType != "" {
		a.OrgType = g.OrgType
	}
	if g.OrgStatus != "" {
		a.OrgStatus = g.OrgStatus
	}
	if g.AvatarURL != "" {
		a.AvatarURL = g.AvatarURL
	}
	if g.Name != "" {
		a.Name = g.Name
	}
	a.BranchID = g.BranchID
	a.BoundBranchID = g.BranchID
	a.Grants(g.Keys)
}

// FromGrant builds the actor for a caller who has no browser session.
//
// The organisation is whatever the grant was resolved for; a grant resolved
// for an organisation the user is not a member of carries no scope and no
// permissions, and so produces an actor that every gate refuses.
func FromGrant(g rbac.Grant) Actor {
	a := Actor{
		UserID:         g.UserID,
		OrganizationID: g.OrganizationID,
		OrgID:          g.OrganizationID,
	}
	ApplyGrant(&a, g)
	return a
}
