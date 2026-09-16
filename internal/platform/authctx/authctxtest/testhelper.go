package authctxtest

import (
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// ActorForRole constructs an Actor populated with declared grants from the RBAC registry.
//
// For platform roles (e.g. "super_admin", "admin", "support", "developer"):
// - Scope is ScopeAdmin, IsStaff is set per role definition, and permissions come from rbac.PlatformRoles().
//
// For organization roles (e.g. "org_owner", "org_manager", "org_accountant", "org_warehouse", "org_sales_rep", "org_pharmacist", "org_employee"):
// - Scope determines whether the role is resolved against ScopeVendor or ScopePharmacy.
// - If scope is empty and role matches an org role, ScopeVendor is used as default.
// - Permissions come from rbac.GrantsFor(role, scope).
func ActorForRole(roleKey string, scope rbac.Scope) authctx.Actor {
	c := rbac.Default()

	// Check platform roles first
	if pRole, ok := rbac.PlatformRole(roleKey); ok {
		perms := rbac.GrantsFor(pRole, rbac.ScopeAdmin)
		if pRole.Owner {
			perms = c.KeysFor(rbac.ScopeAdmin)
		}
		a := authctx.Actor{
			UserID:      1,
			Role:        pRole.Key,
			IsStaff:     pRole.IsStaff,
			IsOwner:     pRole.Owner,
			Scope:       rbac.ScopeAdmin,
			Permissions: perms,
		}
		a.Grants(perms)
		return a
	}

	// Check organization starter roles
	if oRole, ok := rbac.OrganizationRole(roleKey); ok {
		if scope == "" {
			scope = rbac.ScopeVendor
		}
		perms := rbac.GrantsFor(oRole, scope)
		if oRole.Owner {
			perms = c.KeysFor(scope)
		}
		orgType := "customer"
		if scope == rbac.ScopeVendor {
			orgType = "vendor"
		}
		a := authctx.Actor{
			UserID:         1,
			OrganizationID: 1,
			OrgID:          1,
			OrgType:        orgType,
			OrgStatus:      "approved",
			Role:           oRole.Key,
			IsOwner:        oRole.Owner,
			Scope:          scope,
			Permissions:    perms,
		}
		a.Grants(perms)
		return a
	}

	// Fallback for user / customer / job_seeker
	return authctx.Actor{
		UserID: 1,
		Role:   roleKey,
		Scope:  scope,
	}
}

// SyntheticActor creates an Actor with an explicit slice of permissions for tests
// that test gate mechanics directly rather than declared RBAC roles.
func SyntheticActor(userID int64, isStaff bool, perms ...string) authctx.Actor {
	a := authctx.Actor{
		UserID:      userID,
		IsStaff:     isStaff,
		Permissions: perms,
	}
	a.Grants(perms)
	return a
}
