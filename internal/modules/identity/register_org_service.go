package identity

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// RegisterOrganizationInput combines the account and organization details for a
// signup that creates both in one step.
type RegisterOrganizationInput struct {
	Email           string
	Password        string
	NameAr          string
	NameEn          string
	Phone           string
	PhoneVerifiedAt *time.Time
	Language        i18n.Lang
	Timezone        string
	Org             RegisterOrgInput
}

// RegisterOrganization creates a user and their organization in one transaction
// and issues a session scoped to the new organization.
//
// The platform role on identity.users stays 'customer'; the owner membership
// (role_key 'org_owner') is what carries capability. Registration returns the
// user, a session whose ActiveOrgID and OrgType point at the new tenant, and a
// summary of what was created so the caller can route to the right dashboard.
func (s *Service) RegisterOrganization(ctx context.Context, input RegisterOrganizationInput) (*User, *Session, *RegisterOrgResult, error) {
	cleanEmail := NormalizeEmail(input.Email)
	if cleanEmail == "" || !stringsContains(cleanEmail, "@") {
		return nil, nil, nil, apperr.Validation("email.invalid", "A valid email address is required.", nil)
	}

	hash, err := HashPassword(input.Password)
	if err != nil {
		return nil, nil, nil, err
	}

	// Per-type required fields. A pharmacy that omits its licence number must
	// not become a supplier-shaped record; the CHECK constraints and downstream
	// screens rely on the type being complete.
	if err := validateOrgInput(input.Org); err != nil {
		return nil, nil, nil, err
	}

	lang := input.Language
	if lang == "" {
		lang = i18n.Default
	}
	tz := input.Timezone
	if tz == "" {
		tz = "Africa/Cairo"
	}

	user := &User{
		Email:        cleanEmail,
		PasswordHash: hash,
		Name:         i18n.New(input.NameAr, input.NameEn),
		Role:         "user",
		Status:       StatusActive,
		Language:     lang,
		Timezone:        tz,
		Phone:           input.Phone,
		PhoneVerifiedAt: input.PhoneVerifiedAt,
	}

	result, err := s.repo.RegisterOrganization(ctx, user, input.Org)
	if err != nil {
		return nil, nil, nil, err
	}

	permissions, err := s.repo.GetPermissionsForUser(ctx, user.ID, result.OrganizationID)
	if err != nil {
		permissions = []string{}
	}

	sess := &Session{
		UserID:      user.ID,
		PublicID:    user.PublicID,
		Email:       user.Email,
		Role:        user.Role,
		ActiveOrgID: result.OrganizationID,
		OrgType:     result.OrganizationType,
		OrgStatus:   result.OrganizationStatus,
		Permissions: permissions,
	}

	if s.sessionStore != nil {
		if err := s.sessionStore.Create(ctx, sess); err != nil {
			return nil, nil, nil, err
		}
	}

	s.log.InfoContext(ctx, "organization registered",
		"user_id", user.ID, "org_id", result.OrganizationID,
		"org_type", result.OrganizationType, "org_status", result.OrganizationStatus)
	return user, sess, result, nil
}

// ListStaffUserIDs returns the IDs of active platform staff for operational
// notifications (e.g. new registrations awaiting review).
func (s *Service) ListStaffUserIDs(ctx context.Context) ([]int64, error) {
	return s.repo.ListStaffUserIDs(ctx)
}

// validateOrgInput enforces the per-account-type required fields described in
// the registration form.
func validateOrgInput(in RegisterOrgInput) error {
	if in.LegalName == "" {
		return apperr.Validation("org.legal_name_required", i18n.TDefault("w4_mod.w4str_171_171"), nil)
	}
	switch in.Type {
	case OrgTypeVendor, OrgTypeCustomer:
		return nil
	default:
		return apperr.Validation("org.type_invalid", i18n.TDefault("w4_mod.w4str_172_172"), nil)
	}
}
