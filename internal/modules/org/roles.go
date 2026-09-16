package org

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Company role management.
//
// Two rules hold this together, and both are enforced here rather than in the
// handler that calls it:
//
//   - A role belongs to exactly one company. Every method takes the caller's
//     organization id and the repository puts it in the WHERE clause, so a
//     role id from another company reads as missing.
//   - A role may only grant permissions that exist on that company's own
//     dashboard. A vendor owner cannot mint a role holding platform settings
//     however the form is posted, because Restrict drops anything outside the
//     scope before the write.

// RoleInput is a create or update request from a role editor.
type RoleInput struct {
	// OrganizationID is the caller's own company, taken from their session.
	OrganizationID int64
	// Scope is the dashboard the company uses, derived from its type.
	Scope rbac.Scope
	// Key is the stable identifier. It is generated on create and ignored on
	// update — renaming a key would orphan the members holding it.
	Key         string
	Name        i18n.Text
	Description string
	// Permissions is what the form posted. It is filtered, not trusted.
	Permissions []string
	CreatedBy   int64
}

var roleKeyPattern = regexp.MustCompile(`[^a-z0-9_]+`)

// ScopeForOrganization returns the dashboard a company's members use.
func ScopeForOrganization(orgType OrganizationType) (rbac.Scope, error) {
	scope, ok := rbac.TenantScopeFor(string(orgType))
	if !ok {
		return "", apperr.Validation("org.type_has_no_dashboard",
			i18n.TDefault("w4_mod.w4str_227_227"), nil)
	}
	return scope, nil
}

// ListRoles returns one company's roles with their grants.
func (s *Service) ListRoles(ctx context.Context, orgID int64) ([]*Role, error) {
	if orgID <= 0 {
		return nil, apperr.Validation("org.required", "An organization is required.", nil)
	}
	return s.repo.ListRoles(ctx, orgID)
}

// RoleMemberCounts reports how many active members hold each role.
func (s *Service) RoleMemberCounts(ctx context.Context, orgID int64) (map[int64]int, error) {
	if orgID <= 0 {
		return map[int64]int{}, nil
	}
	return s.repo.CountRoleMembers(ctx, orgID)
}

// GetRole returns one of the caller's own roles.
func (s *Service) GetRole(ctx context.Context, orgID, roleID int64) (*Role, error) {
	if orgID <= 0 || roleID <= 0 {
		return nil, apperr.NotFound("role")
	}
	return s.repo.GetRole(ctx, orgID, roleID)
}

// CreateRole adds a company role.
func (s *Service) CreateRole(ctx context.Context, in RoleInput) (*Role, error) {
	if err := validateRoleInput(in); err != nil {
		return nil, err
	}
	role := &Role{
		OrganizationID: in.OrganizationID,
		Key:            roleKeyFrom(in),
		Name:           in.Name,
		Description:    strings.TrimSpace(in.Description),
		Permissions:    rbac.Default().Restrict(in.Permissions, in.Scope),
	}
	if err := s.repo.CreateRole(ctx, role); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "company role created",
		"organization_id", in.OrganizationID, "role_id", role.ID,
		"key", role.Key, "permissions", len(role.Permissions), "by", in.CreatedBy)
	return role, nil
}

// UpdateRole rewrites a role's label and grants.
//
// A system starter role keeps its key and cannot be renamed away from the
// catalogue, but its permissions are fully editable: an owner who wants their
// accountant to also see orders should not have to build a role from nothing.
func (s *Service) UpdateRole(ctx context.Context, roleID int64, in RoleInput) (*Role, error) {
	if err := validateRoleInput(in); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetRole(ctx, in.OrganizationID, roleID)
	if err != nil {
		return nil, err
	}
	if existing.IsOwner {
		// The owner role holds everything by definition; editing it would let
		// an owner lock themselves out of their own company with no way back.
		return nil, apperr.Validation("org.role_is_owner",
			i18n.TDefault("w4_mod.w4str_228_228"), nil)
	}

	existing.Name = in.Name
	existing.Description = strings.TrimSpace(in.Description)
	existing.Permissions = rbac.Default().Restrict(in.Permissions, in.Scope)
	if err := s.repo.UpdateRole(ctx, in.OrganizationID, existing); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "company role updated",
		"organization_id", in.OrganizationID, "role_id", roleID,
		"permissions", len(existing.Permissions), "by", in.CreatedBy)
	return existing, nil
}

// DeleteRole removes a custom role and moves its holders to the employee role.
func (s *Service) DeleteRole(ctx context.Context, orgID, roleID int64) error {
	if orgID <= 0 || roleID <= 0 {
		return apperr.NotFound("role")
	}
	if err := s.repo.DeleteRole(ctx, orgID, roleID); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "company role deleted", "organization_id", orgID, "role_id", roleID)
	return nil
}

// AssignMemberRole puts a member into one of their own company's roles.
func (s *Service) AssignMemberRole(ctx context.Context, orgID, memberID, roleID int64) error {
	if orgID <= 0 || memberID <= 0 || roleID <= 0 {
		return apperr.Validation("org.invalid", i18n.TDefault("w4_mod.w4str_229_229"), nil)
	}
	role, err := s.repo.GetRole(ctx, orgID, roleID)
	if err != nil {
		return err
	}
	if role != nil && (role.IsOwner || role.Key == "org_owner") {
		return apperr.Forbidden("cannot_assign_owner_role", "لا يمكن تعيين رتبة مالك المنشأة للموظفين")
	}
	if err := s.repo.AssignMemberRole(ctx, orgID, memberID, roleID); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "member role assigned",
		"organization_id", orgID, "member_id", memberID, "role_id", roleID)
	return nil
}

func validateRoleInput(in RoleInput) error {
	if in.OrganizationID <= 0 {
		return apperr.Validation("org.required", "An organization is required.", nil)
	}
	if !rbac.ValidScope(in.Scope) {
		return apperr.Validation("org.scope_invalid", i18n.TDefault("w4_mod.w4str_230_230"), nil)
	}
	if strings.TrimSpace(in.Name["ar"]) == "" && strings.TrimSpace(in.Name["en"]) == "" {
		return apperr.Validation("org.role_name_required", i18n.TDefault("w4_mod.w4str_177_177"), nil)
	}
	return nil
}

// roleKeyFrom derives a stable, company-unique key from the requested one or
// from the Arabic name. Keys are lower-case ASCII because they appear in URLs
// and in permission grants; an Arabic name with no ASCII to draw on falls back
// to a generated key rather than to an empty one.
func roleKeyFrom(in RoleInput) string {
	base := strings.ToLower(strings.TrimSpace(in.Key))
	if base == "" {
		base = strings.ToLower(strings.TrimSpace(in.Name["en"]))
	}
	base = roleKeyPattern.ReplaceAllString(strings.ReplaceAll(base, " ", "_"), "")
	base = strings.Trim(base, "_")
	if base == "" {
		return fmt.Sprintf("role_%d", len(in.Name["ar"])+len(in.Permissions)+1)
	}
	if len(base) > 48 {
		base = base[:48]
	}
	// A custom role must never collide with a starter role key, because
	// membership resolution falls back to matching role_key against the
	// company's starter roles.
	if _, isSystem := rbac.OrganizationRole(base); isSystem {
		base = "custom_" + base
	}
	return base
}
