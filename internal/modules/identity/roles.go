package identity

import (
	"context"
	"regexp"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Platform roles — what a super admin builds for their moderators.
//
// The platform shipped with four staff roles hardcoded in three places: a list
// of names in Session.IsStaff, a pair of name comparisons in every permission
// middleware, and four rows in identity.roles. Creating a fifth was therefore
// impossible: a "Finance Moderator" could be inserted, but the audience gate
// did not know the name, so its holders were bounced from /admin/* with a 404.
//
// A role is now a row and nothing else. is_staff decides whether its holders
// reach the admin dashboard, and its grants decide what they see there.

// PlatformRole is a role a user account can hold on the platform.
type PlatformRole struct {
	Key         string    `json:"key"`
	Name        i18n.Text `json:"name"`
	Description string    `json:"description"`
	// IsSystem marks a role the platform ships. Its permissions are managed by
	// the catalogue sync and it cannot be deleted.
	IsSystem bool `json:"is_system"`
	// IsStaff decides whether holders reach /admin/*.
	IsStaff bool `json:"is_staff"`
	// IsOwner marks super_admin, which holds the whole catalogue.
	IsOwner     bool     `json:"is_owner"`
	Permissions []string `json:"permissions"`
	// UserCount is how many accounts currently hold the role.
	UserCount int `json:"user_count"`
}

// PlatformRoleInput is a create or update request from the role editor.
type PlatformRoleInput struct {
	Key         string
	Name        i18n.Text
	Description string
	IsStaff     bool
	Permissions []string
	ActorID     int64
}

var platformRoleKeyPattern = regexp.MustCompile(`[^a-z0-9_]+`)

// ListPlatformRoles returns every platform role with its grants and headcount.
func (s *Service) ListPlatformRoles(ctx context.Context) ([]*PlatformRole, error) {
	return s.repo.ListPlatformRoles(ctx)
}

// GetPlatformRole returns one role by key.
func (s *Service) GetPlatformRole(ctx context.Context, key string) (*PlatformRole, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, apperr.NotFound("role")
	}
	return s.repo.GetPlatformRole(ctx, key)
}

// CreatePlatformRole adds a staff or moderator role.
func (s *Service) CreatePlatformRole(ctx context.Context, in PlatformRoleInput) (*PlatformRole, error) {
	if err := validatePlatformRoleInput(in); err != nil {
		return nil, err
	}
	key := platformRoleKey(in)
	if _, taken := rbac.PlatformRole(key); taken {
		return nil, apperr.Validation("identity.role_key_reserved",
			i18n.TDefault("w4_mod.w4str_173_173"), nil)
	}
	if _, taken := rbac.OrganizationRole(key); taken {
		return nil, apperr.Validation("identity.role_key_reserved",
			i18n.TDefault("w4_mod.w4str_173_173"), nil)
	}

	role := &PlatformRole{
		Key:         key,
		Name:        in.Name,
		Description: strings.TrimSpace(in.Description),
		IsStaff:     in.IsStaff,
		// Restrict to the admin dashboard. A platform role has no business
		// holding vendor or pharmacy permissions: those are resolved from a
		// membership in a specific company, and a platform-wide grant of one
		// would apply to every company at once.
		Permissions: rbac.Default().Restrict(in.Permissions, rbac.ScopeAdmin),
	}
	if err := s.repo.CreatePlatformRole(ctx, role, in.ActorID); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "platform role created",
		"key", role.Key, "is_staff", role.IsStaff,
		"permissions", len(role.Permissions), "actor_id", in.ActorID)
	return role, nil
}

// UpdatePlatformRole rewrites a role's label, staff flag and grants.
//
// The shipped roles are editable too — that is deliberate. An operator who
// wants their "Support" role to also close issues should be able to say so,
// and the alternative is a role nobody can adjust and everybody works around
// by handing out "admin". The one exception is the owner role: it holds
// everything by definition, and editing it is how an operator locks themselves
// out of their own platform.
func (s *Service) UpdatePlatformRole(ctx context.Context, key string, in PlatformRoleInput) (*PlatformRole, error) {
	existing, err := s.GetPlatformRole(ctx, key)
	if err != nil {
		return nil, err
	}
	if existing.IsOwner {
		return nil, apperr.Validation("identity.role_is_owner",
			i18n.TDefault("w4_mod.w4str_174_174"), nil)
	}
	if err := validatePlatformRoleInput(in); err != nil {
		return nil, err
	}

	existing.Name = in.Name
	existing.Description = strings.TrimSpace(in.Description)
	existing.Permissions = rbac.Default().Restrict(in.Permissions, rbac.ScopeAdmin)
	// A shipped role keeps its staff standing: demoting "admin" to non-staff
	// would lock every administrator out of the dashboard in one click.
	if !existing.IsSystem {
		existing.IsStaff = in.IsStaff
	}

	if err := s.repo.UpdatePlatformRole(ctx, existing, in.ActorID); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "platform role updated",
		"key", existing.Key, "permissions", len(existing.Permissions), "actor_id", in.ActorID)
	return existing, nil
}

// DeletePlatformRole removes a custom role.
//
// A role still held by an account is refused rather than cascaded. Moving
// those users to some default would change what they can do without anyone
// deciding it should; the operator is told the count and reassigns them first.
func (s *Service) DeletePlatformRole(ctx context.Context, key string, actorID int64) error {
	existing, err := s.GetPlatformRole(ctx, key)
	if err != nil {
		return err
	}
	if existing.IsSystem {
		return apperr.Validation("identity.role_is_system",
			i18n.TDefault("w4_mod.w4str_175_175"), nil)
	}
	if existing.UserCount > 0 {
		return apperr.Validation("identity.role_in_use",
			i18n.TDefault("w4_mod.w4str_176_176"), nil)
	}
	if err := s.repo.DeletePlatformRole(ctx, key); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "platform role deleted", "key", key, "actor_id", actorID)
	return nil
}

// AssignPlatformRole puts a user account into a platform role and ends their
// sessions, so the new authority takes effect on their next request rather
// than whenever their cookie happens to expire.
func (s *Service) AssignPlatformRole(ctx context.Context, userID int64, key string, actorID int64) error {
	role, err := s.GetPlatformRole(ctx, key)
	if err != nil {
		return err
	}
	if err := s.repo.AdminAssignRole(ctx, userID, role.Key, actorID); err != nil {
		return err
	}
	if s.sessionStore != nil {
		if err := s.sessionStore.DeleteAllForUser(ctx, userID); err != nil {
			s.log.ErrorContext(ctx, "assigned role but could not revoke sessions",
				"error", err, "user_id", userID, "role", role.Key)
			return err
		}
	}
	s.log.InfoContext(ctx, "platform role assigned",
		"user_id", userID, "role", role.Key, "actor_id", actorID)
	return nil
}

func validatePlatformRoleInput(in PlatformRoleInput) error {
	if strings.TrimSpace(in.Name["ar"]) == "" && strings.TrimSpace(in.Name["en"]) == "" {
		return apperr.Validation("identity.role_name_required", i18n.TDefault("w4_mod.w4str_177_177"), nil)
	}
	return nil
}

func platformRoleKey(in PlatformRoleInput) string {
	base := strings.ToLower(strings.TrimSpace(in.Key))
	if base == "" {
		base = strings.ToLower(strings.TrimSpace(in.Name["en"]))
	}
	base = platformRoleKeyPattern.ReplaceAllString(strings.ReplaceAll(base, " ", "_"), "")
	base = strings.Trim(base, "_")
	if base == "" {
		base = "moderator"
	}
	if len(base) > 48 {
		base = base[:48]
	}
	return base
}

// StaffUserInput creates a platform administrator or moderator account.
type StaffUserInput struct {
	Email    string
	Password string
	NameAr   string
	NameEn   string
	Phone    string
	// RoleKey must name a staff role. A non-staff role would create an
	// account that cannot reach the dashboard it was made for.
	RoleKey string
	ActorID int64
}

// CreateStaffUser creates a moderator or administrator account and puts it
// straight into a staff role.
//
// This is the missing half of the roles feature: a super admin could define a
// "Finance Moderator" role and had no way to create anyone to hold it. Signing
// up through the public form produces an ordinary account with no organization,
// which is not what a moderator is.
//
// The role must already exist and must be marked staff. Refusing a non-staff
// role here rather than creating the account and letting the audience gate
// bounce them is the difference between an error an operator can act on and a
// new colleague who cannot log in for reasons nobody can see.
func (s *Service) CreateStaffUser(ctx context.Context, in StaffUserInput) (*User, error) {
	email := NormalizeEmail(in.Email)
	if email == "" || !stringsContains(email, "@") {
		return nil, apperr.Validation("email.invalid", i18n.TDefault("w4_mod.w4str_178_178"), nil)
	}
	if len(in.Password) < 8 {
		return nil, apperr.Validation("password.too_short",
			i18n.TDefault("w4_mod.8_179"), nil)
	}
	nameAr := strings.TrimSpace(in.NameAr)
	if nameAr == "" {
		return nil, apperr.Validation("identity.name_required", i18n.TDefault("w4_mod.w4str_180_180"), nil)
	}

	role, err := s.GetPlatformRole(ctx, strings.TrimSpace(in.RoleKey))
	if err != nil {
		return nil, err
	}
	if !role.IsStaff {
		return nil, apperr.Validation("identity.role_not_staff",
			i18n.TDefault("w4_mod.w4str_181_181"), nil)
	}

	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	nameEn := strings.TrimSpace(in.NameEn)
	if nameEn == "" {
		nameEn = nameAr
	}

	user := &User{
		Email:        email,
		PasswordHash: hash,
		Name:         i18n.New(nameAr, nameEn),
		Role:         role.Key,
		Status:       StatusActive,
		Phone:        strings.TrimSpace(in.Phone),
		Language:     i18n.AR,
		Timezone:     "Africa/Cairo",
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	// The security record is what the login path reads for attempt counting
	// and lockout; an account without one behaves differently from every other.
	if err := s.repo.UpsertSecurity(ctx, &UserSecurity{UserID: user.ID}); err != nil {
		s.log.ErrorContext(ctx, "created staff account but could not initialise its security record",
			"error", err, "user_id", user.ID)
	}

	s.log.InfoContext(ctx, "staff account created",
		"user_id", user.ID, "email", email, "role", role.Key, "actor_id", in.ActorID)
	return user, nil
}
