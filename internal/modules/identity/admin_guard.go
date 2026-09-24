package identity

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// ordinaryRoles are account roles that carry no platform authority: the
// current "user" and "job_seeker" plus the legacy values imported from Laravel.
var ordinaryRoles = map[string]bool{
	"": true, "user": true, RoleJobSeeker: true,
	RoleSupplier: true, RoleVendor: true, RolePharmacy: true,
	RoleEmployer: true, RoleIndividual: true, RoleCustomer: true,
}

// IsPrivilegedRole reports whether a platform role carries staff authority.
// Seeded roles answer from the RBAC catalogue; a custom role created in the
// database is treated as staff, which fails toward requiring the owner.
func IsPrivilegedRole(role string) bool {
	if def, ok := rbac.PlatformRole(role); ok {
		return def.IsStaff || def.Owner
	}
	return !ordinaryRoles[role]
}

// guardPrivilegedTarget refuses an admin action on a staff account unless the
// actor is the platform owner.
//
// identity.user.update is held by the starter Administrator role, which is
// deliberately denied role assignment. Without this check that permission
// could reset the owner's password or second factor and so take the owner
// account — the very escalation withholding role assignment was meant to stop.
// An actor acting on their own account is allowed.
func (s *Service) guardPrivilegedTarget(ctx context.Context, targetID, actorID int64) error {
	if targetID == actorID && actorID > 0 {
		return nil
	}
	target, err := s.repo.GetUserByID(ctx, targetID)
	if err != nil {
		return err
	}
	if !IsPrivilegedRole(target.Role) {
		return nil
	}
	return s.requireOwnerActor(ctx, actorID)
}

// requireOwnerActor refuses unless actorID belongs to an active super_admin.
func (s *Service) requireOwnerActor(ctx context.Context, actorID int64) error {
	if actorID > 0 {
		if actor, err := s.repo.GetUserByID(ctx, actorID); err == nil && actor != nil &&
			actor.Role == RoleSuperAdmin && actor.Status == StatusActive {
			return nil
		}
	}
	return apperr.Forbidden("identity.privileged_target",
		"Only the platform owner can change a staff account.")
}
