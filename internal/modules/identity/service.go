package identity

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Service encapsulates authentication and identity business workflows.
type Service struct {
	repo         Repository
	sessionStore *SessionStore
	resolver     *rbac.Resolver
	log          *slog.Logger
	userCacheMu  sync.RWMutex
	userCache    map[int64]userCacheEntry
	userCacheTTL time.Duration
}

// SetPermissionResolver supplies the shared permission resolver.
func (s *Service) SetPermissionResolver(r *rbac.Resolver) { s.resolver = r }

// resolvePermissions returns the caller's effective permissions.
func (s *Service) resolvePermissions(ctx context.Context, userID, orgID int64) []string {
	perms, _ := s.resolveGrant(ctx, userID, orgID)
	return perms
}

// resolveGrant returns the caller's permissions and whether their platform role reaches admin.
func (s *Service) resolveGrant(ctx context.Context, userID, orgID int64) ([]string, bool) {
	if s.resolver != nil {
		if grant, err := s.resolver.Resolve(ctx, userID, orgID); err == nil {
			return grant.Keys, grant.IsStaff
		} else {
			s.log.ErrorContext(ctx, "could not resolve permissions at login",
				"error", err, "user_id", userID, "organization_id", orgID)
		}
	}
	perms, err := s.repo.GetPermissionsForUser(ctx, userID, orgID)
	if err != nil {
		return []string{}, false
	}
	return perms, false
}

// NewService creates a new Identity Service.
func NewService(repo Repository, sessionStore *SessionStore, log *slog.Logger) *Service {
	return &Service{
		repo:         repo,
		sessionStore: sessionStore,
		log:          log,
		userCache:    make(map[int64]userCacheEntry),
		userCacheTTL: 30 * time.Second,
	}
}

// RegisterInput captures parameters required for creating an account.
type RegisterInput struct {
	Email    string    `json:"email"`
	Password string    `json:"password"`
	NameAr   string    `json:"name_ar"`
	NameEn   string    `json:"name_en"`
	Role     string    `json:"role,omitempty"`
	Language i18n.Lang `json:"language,omitempty"`
	Timezone string    `json:"timezone,omitempty"`
	Phone           string     `json:"phone,omitempty"`
	PhoneVerifiedAt *time.Time `json:"phone_verified_at,omitempty"`
}

// LoginInput captures authentication parameters.
type LoginInput struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	OrgID     int64  `json:"org_id,omitempty"`
	IP        string `json:"-"`
	UserAgent string `json:"-"`
}

// LoginResult contains the output of a successful login or an MFA prompt.
type LoginResult struct {
	User        *User    `json:"user"`
	Session     *Session `json:"session,omitempty"`
	RequiresMFA bool     `json:"requires_mfa"`
}

// Register creates a new user account and sets up security tracking.
func (s *Service) Register(ctx context.Context, input RegisterInput) (*User, *Session, error) {
	cleanEmail := NormalizeEmail(input.Email)
	if cleanEmail == "" || !stringsContains(cleanEmail, "@") {
		return nil, nil, apperr.Validation("email.invalid", "A valid email address is required.", nil)
	}

	hash, err := HashPassword(input.Password)
	if err != nil {
		return nil, nil, err
	}

	role := input.Role
	if role != "support" && role != "admin" && role != "super_admin" && role != "developer" && role != RoleJobSeeker {
		role = "user"
	}

	lang := input.Language
	if lang == "" {
		lang = i18n.Default
	}

	tz := input.Timezone
	if tz == "" {
		tz = "Africa/Cairo"
	}

	status := StatusActive
	if role == RoleJobSeeker {
		status = StatusPending
	}

	user := &User{
		Email:        cleanEmail,
		PasswordHash: hash,
		Name:         i18n.New(input.NameAr, input.NameEn),
		Role:         role,
		Status:       status,
		Language:     lang,
		Timezone:        tz,
		Phone:           input.Phone,
		PhoneVerifiedAt: input.PhoneVerifiedAt,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, nil, err
	}

	// Initialize security record
	sec := &UserSecurity{
		UserID:        user.ID,
		LoginAttempts: 0,
	}
	_ = s.repo.UpsertSecurity(ctx, sec)

	if user.Status == StatusPending {
		s.log.InfoContext(ctx, "user registered pending approval", "user_id", user.ID, "email", user.Email, "role", user.Role)
		return user, nil, nil
	}

	// Issue initial session
	permissions := s.resolvePermissions(ctx, user.ID, 0)

	sess := &Session{
		UserID:      user.ID,
		PublicID:    user.PublicID,
		Email:       user.Email,
		Role:        user.Role,
		Permissions: permissions,
	}

	if s.sessionStore != nil {
		if err := s.sessionStore.Create(ctx, sess); err != nil {
			return nil, nil, err
		}
	}

	s.log.InfoContext(ctx, "user registered", "user_id", user.ID, "email", user.Email, "role", user.Role)
	return user, sess, nil
}

// Login authenticates a user against credentials, lockouts, and MFA requirements.
func (s *Service) Login(ctx context.Context, input LoginInput) (*LoginResult, error) {
	now := time.Now().UTC()
	cleanEmail := NormalizeEmail(input.Email)

	user, err := s.repo.GetUserByEmail(ctx, cleanEmail)
	if err != nil {
		if errors.Is(err, apperr.NotFound("user")) || apperr.KindOf(err) == apperr.KindNotFound {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Check login security and lockout
	sec, err := s.repo.GetSecurity(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	if sec.IsLocked(now) {
		s.log.WarnContext(ctx, "login attempt on locked account", "user_id", user.ID, "locked_until", sec.LockedUntil)
		return nil, apperr.Forbidden("auth.locked", "Account is temporarily locked due to multiple failed login attempts. Please try again later.")
	}

	if err := user.ValidateLogin(); err != nil {
		return nil, err
	}

	if !CheckPassword(user.PasswordHash, input.Password) {
		sec.RecordFailedAttempt(now)
		_ = s.repo.UpsertSecurity(ctx, sec)
		s.log.WarnContext(ctx, "failed login attempt", "user_id", user.ID, "attempts", sec.LoginAttempts)
		return nil, ErrInvalidCredentials
	}

	// Password is correct; reset attempts
	sec.ResetAttempts(now, input.IP, input.UserAgent)
	_ = s.repo.UpsertSecurity(ctx, sec)

	// Check MFA
	mfa, err := s.repo.GetMFA(ctx, user.ID)
	if err == nil && mfa != nil && mfa.Enabled {
		return &LoginResult{
			User:        user,
			RequiresMFA: true,
		}, nil
	}

	// Resolve organization context & permissions
	var orgID int64
	var orgType, orgStatus string
	if input.OrgID > 0 {
		belongs, err := s.repo.UserBelongsToOrg(ctx, user.ID, input.OrgID)
		if err != nil {
			s.log.WarnContext(ctx, "verify user belongs to org", "user_id", user.ID, "org_id", input.OrgID, "error", err)
		} else if belongs {
			orgID = input.OrgID
		}
	}
	// No organization named - and the web sign-in form has no field for one, so
	// this is the usual case. Fall back to the user's own membership rather
	// than leaving the session on organization 0, which every tenant-scoped
	// query then filters against and finds nothing.
	if orgID == 0 {
		if o, t, st, err := s.repo.DefaultOrgInfoForUser(ctx, user.ID); err == nil {
			orgID, orgType, orgStatus = o, t, st
		} else {
			s.log.WarnContext(ctx, "could not resolve default organization at login",
				"error", err, "user_id", user.ID)
		}
	}

	if orgType != "" {
		normType, wasLegacy := NormalizeOrgType(orgType)
		if wasLegacy {
			s.log.WarnContext(ctx, "legacy organization type normalized at login",
				"original", orgType, "normalized", normType, "user_id", user.ID)
		}
		orgType = normType
	}

	permissions, staffRole := s.resolveGrant(ctx, user.ID, orgID)

	// Resolve dynamic plan concurrency limit:
	maxSessions := 3
	if maxS, _, _, err := s.repo.GetOrgPlanLimits(ctx, orgID); err == nil && maxS > 0 {
		maxSessions = maxS
	} else if sec.MaxLoginSessions != nil && *sec.MaxLoginSessions > 0 {
		maxSessions = *sec.MaxLoginSessions
	}

	dev := ParseUserAgentDevice(input.UserAgent, input.IP)

	sess := &Session{
		UserID:           user.ID,
		PublicID:         user.PublicID,
		Email:            user.Email,
		Role:             user.Role,
		ActiveOrgID:      orgID,
		OrgType:          orgType,
		OrgStatus:        orgStatus,
		StaffRole:        staffRole,
		Permissions:      permissions,
		IP:               input.IP,
		UserAgent:        input.UserAgent,
		DeviceName:       dev.DeviceName,
		DeviceType:       dev.DeviceType,
		Browser:          dev.Browser,
		OS:               dev.OS,
		Icon:             dev.Icon,
		MaxLoginSessions: &maxSessions,
	}

	if s.sessionStore != nil {
		if err := s.sessionStore.Create(ctx, sess); err != nil {
			return nil, err
		}
	}

	s.log.InfoContext(ctx, "user logged in", "user_id", user.ID, "active_org", orgID, "device", dev.DeviceName, "max_sessions", maxSessions)
	return &LoginResult{
		User:    user,
		Session: sess,
	}, nil
}

// ChangePassword verifies current password and updates to the new hashed password.
func (s *Service) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !CheckPassword(user.PasswordHash, currentPassword) {
		return apperr.Validation("auth.invalid_password", i18n.TDefault("w4_mod.w4str_182_182"), nil)
	}
	if len(newPassword) < 8 {
		return apperr.Validation("auth.weak_password", i18n.TDefault("w4_mod.8_183"), nil)
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	user.PasswordHash = hash
	user.UpdatedAt = time.Now()
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}
	if s.sessionStore != nil {
		_ = s.sessionStore.DeleteAllForUser(ctx, userID)
	}
	s.log.InfoContext(ctx, "user password updated", "user_id", userID)
	return nil
}

// Logout terminates a session.
func (s *Service) Logout(ctx context.Context, token string) error {

	if s.sessionStore == nil {
		return nil
	}
	return s.sessionStore.Delete(ctx, token)
}

// ListSessions returns the user's active sessions, newest first.
func (s *Service) ListSessions(ctx context.Context, userID int64) ([]*Session, error) {
	if s.sessionStore == nil {
		return nil, nil
	}
	return s.sessionStore.ListForUser(ctx, userID)
}

// ListOrgSessions returns all active sessions across an entire organization, newest first.
func (s *Service) ListOrgSessions(ctx context.Context, orgID int64) ([]*Session, error) {
	if s.sessionStore == nil {
		return nil, nil
	}
	return s.sessionStore.ListForOrg(ctx, orgID)
}

// DefaultOrgInfoForUser returns the default organization details for a user.
func (s *Service) DefaultOrgInfoForUser(ctx context.Context, userID int64) (int64, string, string, error) {
	return s.repo.DefaultOrgInfoForUser(ctx, userID)
}

