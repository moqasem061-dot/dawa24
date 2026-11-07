package identity

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
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
		if grant, err := s.resolver.Resolve(ctx, userID, orgID); err == nil { // nolint:errswallow: the else branch logs it
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
	Email    string `json:"email"`
	Password string `json:"password"`
	NameAr   string `json:"name_ar"`
	NameEn   string `json:"name_en"`
	// Role, PhoneVerifiedAt and EmailVerifiedAt are set only by server code.
	// They are excluded from JSON so a self-service caller cannot choose a
	// platform role or claim a verified contact.
	Role string `json:"-"`
	// MustChangePassword is set by server code only: an administrator created
	// this account with a password of the administrator's choosing.
	MustChangePassword bool       `json:"-"`
	Language           i18n.Lang  `json:"language,omitempty"`
	Timezone           string     `json:"timezone,omitempty"`
	Phone              string     `json:"phone,omitempty"`
	PhoneVerifiedAt    *time.Time `json:"-"`
	EmailVerifiedAt    *time.Time `json:"-"`
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

	// Registration never grants a platform role. Staff accounts are created
	// only through CreateStaffUser, which requires identity.admin_role.assign.
	role := "user"
	if input.Role == RoleJobSeeker {
		role = RoleJobSeeker
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
		Email:              cleanEmail,
		PasswordHash:       hash,
		Name:               i18n.New(input.NameAr, input.NameEn),
		Role:               role,
		Status:             status,
		Language:           lang,
		Timezone:           tz,
		Phone:              input.Phone,
		PhoneVerifiedAt:    input.PhoneVerifiedAt,
		EmailVerifiedAt:    input.EmailVerifiedAt,
		MustChangePassword: input.MustChangePassword,
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
		UserID:             user.ID,
		PublicID:           user.PublicID,
		Email:              user.Email,
		Role:               user.Role,
		Permissions:        permissions,
		MustChangePassword: user.MustChangePassword,
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
// The password is checked before anything about the account is revealed: a
// caller who does not know the password learns nothing about status or lockout.
func (s *Service) Login(ctx context.Context, input LoginInput) (*LoginResult, error) {
	now := time.Now().UTC()
	cleanEmail := NormalizeEmail(input.Email)

	user, err := s.repo.GetUserByEmail(ctx, cleanEmail)
	if err != nil {
		if errors.Is(err, apperr.NotFound("user")) || apperr.KindOf(err) == apperr.KindNotFound {
			// Spend the same bcrypt time as a real check, so the response time
			// does not show whether the email is registered.
			CheckPassword(unknownUserHash(), input.Password)
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Check login security and lockout
	sec, err := s.repo.GetSecurity(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	// An account without a stored hash still spends the bcrypt time, and never matches.
	hash := user.PasswordHash
	if hash == "" {
		hash = unknownUserHash()
	}
	passwordOK := CheckPassword(hash, input.Password) && user.PasswordHash != ""

	if sec.IsLocked(now) {
		// Wrong guesses during a lock are not recorded, so they do not extend it.
		if !passwordOK {
			return nil, ErrInvalidCredentials
		}
		s.log.WarnContext(ctx, "login attempt on locked account", "user_id", user.ID, "locked_until", sec.LockedUntil)
		return nil, apperr.Forbidden("auth.locked", "Account is temporarily locked due to multiple failed login attempts. Please try again later.")
	}

	if !passwordOK {
		sec.RecordFailedAttempt(now)
		_ = s.repo.UpsertSecurity(ctx, sec)
		s.log.WarnContext(ctx, "failed login attempt", "user_id", user.ID, "attempts", sec.LoginAttempts)
		return nil, ErrInvalidCredentials
	}

	if err := user.ValidateLogin(); err != nil {
		return nil, err
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
		if o, t, st, err := s.repo.DefaultOrgInfoForUser(ctx, user.ID); err == nil { // nolint:errswallow: the else branch logs it
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
		UserID:             user.ID,
		PublicID:           user.PublicID,
		Email:              user.Email,
		Role:               user.Role,
		ActiveOrgID:        orgID,
		OrgType:            orgType,
		OrgStatus:          orgStatus,
		StaffRole:          staffRole,
		MustChangePassword: user.MustChangePassword,
		Permissions:        permissions,
		IP:                 input.IP,
		UserAgent:          input.UserAgent,
		DeviceName:         dev.DeviceName,
		DeviceType:         dev.DeviceType,
		Browser:            dev.Browser,
		OS:                 dev.OS,
		Icon:               dev.Icon,
		MaxLoginSessions:   &maxSessions,
	}

	if s.sessionStore != nil {
		if err := s.sessionStore.Create(ctx, sess); err != nil {
			return nil, err
		}
	}

	var pOrgID *int64
	if orgID > 0 {
		pOrgID = &orgID
	}
	_ = s.RecordAudit(ctx, database.AuditEntry{
		OrganizationID: pOrgID,
		ActorUserID:    user.ID,
		Action:         "user.login",
		EntityType:     "user",
		EntityID:       strconv.FormatInt(user.ID, 10),
		IP:             input.IP,
		After: map[string]any{
			"device":      dev.DeviceName,
			"device_type": dev.DeviceType,
			"browser":     dev.Browser,
			"os":          dev.OS,
			"ip":          input.IP,
			"login_time":  now.Format(time.RFC3339),
		},
	})

	s.log.InfoContext(ctx, "user logged in", "user_id", user.ID, "active_org", orgID, "device", dev.DeviceName, "max_sessions", maxSessions)
	return &LoginResult{
		User:    user,
		Session: sess,
	}, nil
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
