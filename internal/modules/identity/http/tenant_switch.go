package http

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

// switchActorOrg rebuilds every authorization field of actor for orgID after
// ResolveTenant has confirmed membership.
//
// Assigning only actor.Permissions was not enough: Can* reads the permission
// set Grants() built for the session's organization, and IsOwner, Scope,
// BranchID and OrgID also stayed behind. A user who owns one company and is a
// junior employee of another then acted inside the second with the first's
// owner authority.
func switchActorOrg(ctx context.Context, service *identity.Service, actor authctx.Actor,
	userID, orgID int64, orgType, orgStatus string, perms []string) authctx.Actor {
	actor.OrganizationID = orgID
	actor.OrgID = orgID
	actor.OrgType = orgType
	actor.OrgStatus = orgStatus

	if g, ok := service.ResolveGrant(ctx, userID, orgID); ok {
		authctx.ApplyGrant(&actor, g)
		return actor
	}

	// No resolver: keep only what the membership lookup established and drop
	// every elevation that belonged to the previous organization.
	actor.IsOwner = false
	actor.BranchID = nil
	actor.BoundBranchID = nil
	actor.Scope = ""
	if scope, ok := rbac.TenantScopeFor(orgType); ok {
		actor.Scope = scope
	}
	actor.Grants(perms)
	return actor
}
