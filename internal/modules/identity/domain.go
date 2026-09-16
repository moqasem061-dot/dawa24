package identity

import (
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// User status enum matching PostgreSQL CHECK constraint in 002_identity.
type UserStatus string

const (
	StatusActive    UserStatus = "active"
	StatusInactive  UserStatus = "inactive"
	StatusSuspended UserStatus = "suspended"
	StatusPending   UserStatus = "pending"
	StatusDeleted   UserStatus = "deleted"
)

const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleSupplier   = "supplier"
	RoleVendor     = "vendor"
	RolePharmacy   = "pharmacy"
	RoleEmployer   = "employer"
	RoleIndividual = "individual"
	RoleCustomer   = "customer"
	RoleJobSeeker  = "job_seeker"
)

// MaxFailedLoginsBeforeLockout defines when an account is temporarily locked.
const (
	MaxFailedLoginsBeforeLockout = 5
	DefaultLockoutDuration       = 15 * time.Minute
)

// User represents the core identity entity.
type User struct {
	ID              int64      `json:"id"`
	PublicID        string     `json:"public_id"`
	Email           string     `json:"email"`
	PasswordHash    string     `json:"-"`
	Name            i18n.Text  `json:"name"`
	Role            string     `json:"role"`
	Status          UserStatus `json:"status"`
	Language        i18n.Lang  `json:"language"`
	Timezone        string     `json:"timezone"`
	Phone           string     `json:"phone,omitempty"`
	AvatarURL       string     `json:"avatar_url,omitempty"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	PhoneVerifiedAt *time.Time `json:"phone_verified_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}

// AccountDeletionRequest represents a formal user request to delete their account.
type AccountDeletionRequest struct {
	ID               int64      `json:"id"`
	UserID           int64      `json:"user_id"`
	UserName         string     `json:"user_name"`
	UserEmail        string     `json:"user_email"`
	UserRole         string     `json:"user_role"`
	OrganizationID   *int64     `json:"organization_id,omitempty"`
	OrganizationName string     `json:"organization_name,omitempty"`
	Reason           string     `json:"reason"`
	Status           string     `json:"status"` // "pending", "approved", "rejected"
	AdminNotes       string     `json:"admin_notes"`
	ReviewedBy       *int64     `json:"reviewed_by,omitempty"`
	ReviewedAt       *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// UserOrgMembership represents an organization context the user belongs to.
type UserOrgMembership struct {
	OrganizationID int64     `json:"organization_id"`
	OrgName        i18n.Text `json:"org_name"`
	OrgType        string    `json:"org_type"`
	OrgStatus      string    `json:"org_status"`
	RoleKey        string    `json:"role_key"`
	IsActive       bool      `json:"is_active"`
}

// UserSecurity tracks login security, attempts, and lockouts.
type UserSecurity struct {
	UserID              int64      `json:"user_id"`
	LoginAttempts       int        `json:"login_attempts"`
	LockedUntil         *time.Time `json:"locked_until,omitempty"`
	LastLoginAt         *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP         string     `json:"last_login_ip,omitempty"`
	LastUserAgent       string     `json:"last_user_agent,omitempty"`
	LastPasswordChange  *time.Time `json:"last_password_change,omitempty"`
	PasswordChangeCount int        `json:"password_change_count"`
	MaxLoginSessions    *int       `json:"max_login_sessions,omitempty"`
}

// UserMFA holds two-factor authentication state.
type UserMFA struct {
	UserID        int64      `json:"user_id"`
	TOTPSecret    []byte     `json:"-"`
	RecoveryCodes []byte     `json:"-"`
	Enabled       bool       `json:"enabled"`
	ConfirmedAt   *time.Time `json:"confirmed_at,omitempty"`
}

// Role represents a platform or organization RBAC role.
type Role struct {
	Key         string    `json:"key"`
	Name        i18n.Text `json:"name"`
	Scope       string    `json:"scope"` // "platform" or "organization"
	IsSystem    bool      `json:"is_system"`
	Description string    `json:"description,omitempty"`
}

// Permission represents a fine-grained capability.
type Permission struct {
	Key         string    `json:"key"`
	Name        i18n.Text `json:"name"`
	Module      string    `json:"module"`
	Description string    `json:"description,omitempty"`
}

// ValidatePassword checks that the password meets security requirements:
// at least 8 characters, with uppercase, lowercase, digit, and special character.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return apperr.Validation("password.too_short", i18n.TDefault("w4_mod.8_167"), nil)
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		return apperr.Validation("password.weak", i18n.TDefault("w4_mod.w4str_168_168"), nil)
	}
	return nil
}

// HashPassword hashes a plain-text password using bcrypt (cost 10).
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", apperr.Validation("password.too_short", "Password must be at least 8 characters long.", nil)
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", apperr.Internal(err)
	}
	return string(bytes), nil
}

// CheckPassword verifies a plaintext password against a bcrypt hash ($2y$ or $2a$).
func CheckPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// IsLocked checks whether the account is currently locked out.
func (s *UserSecurity) IsLocked(now time.Time) bool {
	if s == nil || s.LockedUntil == nil {
		return false
	}
	return s.LockedUntil.After(now)
}

// RecordFailedAttempt increments the failed attempt count and applies a lockout if threshold is exceeded.
func (s *UserSecurity) RecordFailedAttempt(now time.Time) {
	if s == nil {
		return
	}
	if s.LockedUntil != nil && now.After(*s.LockedUntil) {
		s.LoginAttempts = 0
		s.LockedUntil = nil
	}
	s.LoginAttempts++
	if s.LoginAttempts >= MaxFailedLoginsBeforeLockout {
		lockout := now.Add(DefaultLockoutDuration)
		s.LockedUntil = &lockout
	}
}

// ResetAttempts clears the failed attempt counter upon a successful authentication.
func (s *UserSecurity) ResetAttempts(now time.Time, ip, userAgent string) {
	if s == nil {
		return
	}
	s.LoginAttempts = 0
	s.LockedUntil = nil
	s.LastLoginAt = &now
	s.LastLoginIP = ip
	s.LastUserAgent = userAgent
}

// ValidateLogin checks if the user's status permits logging in.
func (u *User) ValidateLogin() error {
	if u.DeletedAt != nil {
		return apperr.NotFound("user")
	}
	switch u.Status {
	case StatusActive:
		return nil
	case StatusSuspended:
		return apperr.Forbidden("auth.suspended", "Account has been suspended. Please contact support.")
	case StatusPending:
		return apperr.Forbidden("auth.pending", "Account approval is pending.")
	case StatusInactive:
		return apperr.Forbidden("auth.inactive", "Account is inactive.")
	default:
		return apperr.Unauthorized()
	}
}

// NormalizeEmail cleans and lowercases emails for consistent lookup.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ErrInvalidCredentials is a reusable authentication failure error.
var ErrInvalidCredentials = apperr.New(apperr.KindUnauthorized, "auth.invalid_credentials", "Invalid email or password.")

// UserAddress represents a saved shipping/billing address.
type UserAddress struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Title     string    `json:"title"`
	Recipient string    `json:"recipient"`
	Phone     string    `json:"phone"`
	CityID    int64     `json:"city_id"`
	Address   string    `json:"address"`
	Building  string    `json:"building,omitempty"`
	Floor     string    `json:"floor,omitempty"`
	Apartment string    `json:"apartment,omitempty"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserFavorite represents a bookmarked product.
type UserFavorite struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	ProductID int64     `json:"product_id"`
	CreatedAt time.Time `json:"created_at"`
}

// UserAddressHistory is one append-only record of an address change.
type UserAddressHistory struct {
	ID        int64          `json:"id"`
	UserID    int64          `json:"user_id"`
	AddressID *int64         `json:"address_id,omitempty"`
	Event     string         `json:"event"` // created, updated, deleted
	Snapshot  map[string]any `json:"snapshot"`
	ChangedAt time.Time      `json:"changed_at"`
}

// MeResponse represents the authenticated user's profile and active session context.
type MeResponse struct {
	User        *User    `json:"user"`
	ActiveOrgID *int64   `json:"active_org_id,omitempty"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

// Organization account types accepted at registration. These mirror
// internal/modules/org but are re-declared here so identity can validate the
// input without importing another module (module boundary: modules/A must not
// import modules/B).
//
// Two types only (Rebuild V2 rule 1): customer (صيدلية) and vendor (مورّد).
// Every legacy value — pharmacy, chain_pharmacy, supplier, company, agency,
// individual — maps onto these two. Platform admin is staff, not a type.
const (
	OrgTypeVendor   = "vendor"
	OrgTypeCustomer = "customer"
)

// NormalizeOrgType maps legacy organization type values ('pharmacy', 'chain_pharmacy',
// 'individual', 'supplier', 'company', 'agency') onto the two canonical values ('customer', 'vendor').
func NormalizeOrgType(t string) (string, bool) {
	switch t {
	case OrgTypeCustomer, OrgTypeVendor:
		return t, false
	case "pharmacy", "chain_pharmacy", "individual":
		return OrgTypeCustomer, true
	case "supplier", "company", "agency":
		return OrgTypeVendor, true
	default:
		return t, false
	}
}

// RegisterOrgInput carries the organization details a signup collects, per the
// account type chosen in step 1 of the registration form.
type RegisterOrgInput struct {
	Type               string `json:"type"`
	LegalName          string `json:"legal_name"`
	TradeNameAr        string `json:"trade_name_ar"`
	TradeNameEn        string `json:"trade_name_en"`
	CommercialRegister string `json:"commercial_register"`
	TaxNumber          string `json:"tax_number,omitempty"`
	PharmacistLicense  string `json:"pharmacist_license,omitempty"`
	LicenseDocumentURL string `json:"license_document_url,omitempty"`
	// LicenseOriginalName/MimeType/SizeBytes describe the uploaded license
	// file so the stored document row is previewable (file_url, name, mime).
	// Empty when the registrant attached no file.
	LicenseOriginalName string   `json:"license_original_name,omitempty"`
	LicenseMimeType     string   `json:"license_mime_type,omitempty"`
	LicenseSizeBytes    int64    `json:"license_size_bytes,omitempty"`
	CityID              *int64   `json:"city_id,omitempty"`
	BranchCount         *int     `json:"branch_count,omitempty"`
	Address             string   `json:"address,omitempty"`
	Latitude            *float64 `json:"latitude,omitempty"`
	Longitude           *float64 `json:"longitude,omitempty"`
	GoogleMapsURL       string   `json:"google_maps_url,omitempty"`
}

// RegisterOrgResult reports what the one-transaction signup created.
type RegisterOrgResult struct {
	OrganizationID       int64  `json:"organization_id"`
	OrganizationPublicID string `json:"organization_public_id"`
	OrganizationType     string `json:"organization_type"`
	OrganizationStatus   string `json:"organization_status"`
}

// UserPreferences holds display and notification preferences.
type UserPreferences struct {
	UserID               int64           `json:"user_id"`
	Theme                string          `json:"theme"` // light, dark, auto
	NotificationChannels map[string]bool `json:"notification_channels"`
	NotificationTopics   map[string]bool `json:"notification_topics"`
	MarketingConsent     bool            `json:"marketing_consent"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// AdminUserFilter specifies criteria for paginated administrative user queries.
type AdminUserFilter struct {
	Role          string     `json:"role,omitempty"`
	Status        string     `json:"status,omitempty"`
	Search        string     `json:"search,omitempty"`
	OrgID         int64      `json:"org_id,omitempty"`
	Type          string     `json:"type,omitempty"`
	MFA           *bool      `json:"mfa,omitempty"`
	LastLoginFrom *time.Time `json:"last_login_from,omitempty"`
	LastLoginTo   *time.Time `json:"last_login_to,omitempty"`
}

// AdminEditUserInput carries fields for full administrative user profile modification.
type AdminEditUserInput struct {
	NameAr     string     `json:"name_ar"`
	NameEn     string     `json:"name_en"`
	Email      string     `json:"email"`
	Phone      string     `json:"phone"`
	AvatarURL  string     `json:"avatar_url"`
	Status     UserStatus `json:"status"`
	NationalID string     `json:"national_id"`
}

// AdminUserStatsResult holds aggregated user metrics for the admin user management screen.
type AdminUserStatsResult struct {
	Total     int `json:"total"`
	Staff     int `json:"staff"`
	Vendor    int `json:"vendor"`
	Customer  int `json:"customer"`
	Active    int `json:"active"`
	Suspended int `json:"suspended"`
}
