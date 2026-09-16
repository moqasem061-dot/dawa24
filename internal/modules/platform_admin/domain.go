// Package platformadmin handles system-wide configurations, geographical master data,
// and platform administration settings.
package platformadmin

import (
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// SystemSetting represents a global system key-value configuration.
type SystemSetting struct {
	Key         string         `json:"key"`
	Value       map[string]any `json:"value"`
	Description string         `json:"description,omitempty"`
	IsPublic    bool           `json:"is_public"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// TempWarehouseLifecycleSettings configures auto-archive and auto-purge retention for temporary warehouses.
type TempWarehouseLifecycleSettings struct {
	AutoArchiveHours   int  `json:"auto_archive_hours"`   // Hours from creation until file is auto-archived (default 720 = 30 days)
	AutoArchiveEnabled bool `json:"auto_archive_enabled"` // Master toggle for auto-archiving
	AutoDeleteDays     int  `json:"auto_delete_days"`     // Days from archiving until permanent deletion of file and rows (default 30 days)
	AutoDeleteEnabled  bool `json:"auto_delete_enabled"`  // Master toggle for auto-deleting
}

// Country represents a supported operating country.
type Country struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	Name      i18n.Text `json:"name"`
	PhoneCode string    `json:"phone_code"`
	Currency  string    `json:"currency"`
	IsActive  bool      `json:"is_active"`
}

// Governorate represents an administrative governorate (المحافظة) within a country.
type Governorate struct {
	ID                   int64     `json:"id"`
	CountryID            int64     `json:"country_id"`
	Name                 i18n.Text `json:"name"`
	Latitude             float64   `json:"latitude"`
	Longitude            float64   `json:"longitude"`
	IsActive             bool      `json:"is_active"`
	CityCount            int       `json:"city_count,omitempty"`
	CoverageRadiusMeters int       `json:"coverage_radius_meters"`
}

// NormalizedRadius clamps a submitted governorate radius.
func (g Governorate) NormalizedRadius() int {
	if g.CoverageRadiusMeters <= 0 {
		return 25000
	}
	return min(max(g.CoverageRadiusMeters, 5000), 100000)
}

// CoverageRadiusKM renders the governorate radius in kilometers.
func (g Governorate) CoverageRadiusKM() float64 {
	return float64(g.NormalizedRadius()) / 1000
}

// City represents an operating subgovernorate / city / district (المدينة / المركز / الحي) within a governorate.
type City struct {
	ID              int64      `json:"id"`
	CountryID       int64      `json:"country_id"`
	GovernorateID   *int64     `json:"governorate_id,omitempty"`
	GovernorateName *i18n.Text `json:"governorate_name,omitempty"`
	Name            i18n.Text  `json:"name"`
	Latitude        float64    `json:"latitude"`
	Longitude       float64    `json:"longitude"`
	IsActive        bool       `json:"is_active"`
	IsCapital       bool       `json:"is_capital"`
	// CoverageRadiusMeters is how far from this city's centre a delivery is
	// still considered to be inside it.
	//
	// It belongs to the city rather than to any vendor. Coverage used to be a
	// single figure a distributor typed once and had applied to every city they
	// selected — five kilometres by default, which reaches four unrelated
	// districts from the centre of حدائق الزيتون and nothing but desert from the
	// centre of أبو سمبل. Nobody can hand-size three hundred and fifty places,
	// so in practice every coverage row carried the default and coverage meant
	// nothing. See migration 167 for how the values are derived.
	CoverageRadiusMeters int `json:"coverage_radius_meters"`
}

// Coverage bounds, stated once. They are the same numbers migration 167 clamps
// to, restated here because a value arriving from an admin form has not been
// through that migration.
const (
	// MinCoverageRadiusMeters is the smallest useful delivery radius: below it a
	// vendor covering a district would miss the far side of it.
	MinCoverageRadiusMeters = 500
	// MaxCoverageRadiusMeters is where a single circle stops being a city. A
	// vendor who needs more than this should be selecting more cities.
	MaxCoverageRadiusMeters = 50000
	// DefaultCoverageRadiusMeters applies to a city with no coordinates, which
	// is the one country-level row the table carries.
	DefaultCoverageRadiusMeters = 3000
)

// NormalizedRadius clamps a submitted radius into the range the column accepts.
func (c City) NormalizedRadius() int {
	if c.CoverageRadiusMeters <= 0 {
		return DefaultCoverageRadiusMeters
	}
	return min(max(c.CoverageRadiusMeters, MinCoverageRadiusMeters), MaxCoverageRadiusMeters)
}

// CoverageRadiusKM renders the radius the way a coverage screen states it.
func (c City) CoverageRadiusKM() float64 {
	return float64(c.NormalizedRadius()) / 1000
}

// Validate ensures setting keys are non-empty.
func (s *SystemSetting) Validate() error {
	if s.Key == "" {
		return apperr.Validation("setting.key_required", "Setting key is required.", nil)
	}
	return nil
}

// Currency represents a supported monetary currency.
type Currency struct {
	ID              int64     `json:"id"`
	Code            string    `json:"code"`
	Name            i18n.Text `json:"name"`
	Symbol          string    `json:"symbol"`
	ExchangeRateEGP float64   `json:"exchange_rate_egp"`
	IsActive        bool      `json:"is_active"`
	IsDefault       bool      `json:"is_default"`
}

// Language represents a supported UI locale language.
type Language struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Dir       string `json:"dir"` // rtl or ltr
	IsActive  bool   `json:"is_active"`
	IsDefault bool   `json:"is_default"`
}

// ContactMessage represents a public contact inquiry form submission.
type ContactMessage struct {
	ID        int64     `json:"id"`
	PublicID  string    `json:"public_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone,omitempty"`
	Subject   string    `json:"subject"`
	Message   string    `json:"message"`
	Status    string    `json:"status"` // unread, read, resolved
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	UserID    *int64    `json:"user_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Document represents an organization uploaded official verification document.
type Document struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID *int64    `json:"organization_id,omitempty"`
	Title          string    `json:"title"`
	DocumentType   string    `json:"document_type"`
	StorageKey     string    `json:"storage_key"`
	CreatedAt      time.Time `json:"created_at"`
}

// ContentBlock is an editable bilingual CMS block (legacy what_in_contents).
type ContentBlock struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`
	Title     i18n.Text `json:"title"`
	Body      i18n.Text `json:"body"`
	Position  string    `json:"position"`
	SortOrder int       `json:"sort_order"`
	IsActive  bool      `json:"is_active"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Visitor is one analytics row: a unique visitor on a unique day.
type Visitor struct {
	ID         int64     `json:"id"`
	VisitorKey string    `json:"visitor_key"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	Browser    string    `json:"browser"`
	Device     string    `json:"device"`
	OS         string    `json:"os"`
	Country    string    `json:"country"`
	City       string    `json:"city"`
	VisitedAt  time.Time `json:"visited_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// VisitorAnalytics is the aggregate view for the admin analytics page.
type VisitorAnalytics struct {
	Total           int
	Today           int
	ByCountry       map[string]int
	ByCity          map[string]int
	ByDevice        map[string]int
	ByOS            map[string]int
	ByBrowser       map[string]int
	TotalPharmacies int
	TotalSuppliers  int
	TotalOrders     int
	TotalProducts   int
	TotalGMV        string
	Recent          []*Visitor
	Page            int
	PerPage         int
}

// AISettings defines configuration for artificial intelligence assistant parameters.
type AISettings struct {
	APIKey       string  `json:"api_key"`       // API secret key
	EndpointURL  string  `json:"endpoint_url"`  // Custom or Gateway URL
	Temperature  float64 `json:"temperature"`   // 0.0 - 1.0
	MaxTokens    int     `json:"max_tokens"`    // default 2048
	SystemPrompt string  `json:"system_prompt"` // Default platform assistant prompt
	IsActive     bool    `json:"is_active"`
}

// GatewaySettings defines configuration for platform API gateways and external integrations.
//
// Two credentials live here and they are not interchangeable. APIKey is the
// administrator credential for the Gateway's /api management surface, used with
// basic auth to register users and mint keys. VirtualKey is a Bearer token the
// Gateway issued for the admin panel's own identity, and is the only thing that
// authenticates a /v1 completion. Sending the first where the second belongs is
// a 401 on every AI call — which is precisely why the AI features looked inert.
type GatewaySettings struct {
	EndpointURL    string `json:"endpoint_url"`    // Gateway Base URL
	Environment    string `json:"environment"`     // production / sandbox
	TimeoutSeconds int    `json:"timeout_seconds"` // Request timeout
	APIKey         string `json:"api_key"`         // admin credential, "user:password" or password
	IsActive       bool   `json:"is_active"`

	// VirtualKey is provisioned automatically from APIKey; an operator never
	// types it.
	VirtualKey string `json:"virtual_key"`
	// AIUserID is the Gateway user the admin panel runs as.
	AIUserID string `json:"ai_user_id"`
	// FastModel and QualityModel let an operator point each capability tier at
	// whatever their Gateway publishes. Empty means the platform default.
	FastModel    string `json:"fast_model"`
	QualityModel string `json:"quality_model"`
	// AIPlanID is the Gateway plan the admin panel identity is created under.
	AIPlanID string `json:"ai_plan_id"`
}

// AdminCredentials splits the stored admin credential into a username and
// password. A value with no colon is a password for the conventional "admin"
// account, which is how the settings screen has always accepted it.
//
// A value that does not look like a Gateway credential yields nothing at all.
// SaveGatewaySettings already refuses to store one, but validating only on the
// way in protects nothing that is already stored — and the production database
// was found holding its own superuser password in this field, which the client
// then sent as basic auth to a third-party host on every call. Returning empty
// here is the containment: the value cannot leave the process no matter which
// caller asks for it, and CanProvision reports false so the caller degrades to
// its fallback instead of authenticating with a database credential.
func (g *GatewaySettings) AdminCredentials() (username, password string) {
	raw := strings.TrimSpace(g.APIKey)
	if raw == "" {
		return "", ""
	}
	if ValidateAdminCredential(raw) != nil {
		return "", ""
	}
	if user, pass, found := strings.Cut(raw, ":"); found {
		return strings.TrimSpace(user), pass
	}
	return "admin", raw
}

// CanProvision reports whether there is enough configuration to talk to the
// Gateway's management API at all.
func (g *GatewaySettings) CanProvision() bool {
	if g == nil || !g.IsActive || strings.TrimSpace(g.EndpointURL) == "" {
		return false
	}
	_, password := g.AdminCredentials()
	return password != ""
}

// SiteSettings defines public website branding, contact info, session timeout, and social media.
type SiteSettings struct {
	SiteName                  string            `json:"site_name"`
	SiteDescription           string            `json:"site_description"`
	LogoURL                   string            `json:"logo_url"`
	LogoDarkURL               string            `json:"logo_dark_url"`
	FaviconURL                string            `json:"favicon_url"`
	ContactEmail              string            `json:"contact_email"`
	SupportEmail              string            `json:"support_email"`
	Phone                     string            `json:"phone"`
	WhatsApp                  string            `json:"whatsapp"`
	Address                   string            `json:"address"`
	SessionIdleTimeoutMinutes int               `json:"session_idle_timeout_minutes"`
	SocialLinks               map[string]string `json:"social_links"` // facebook, twitter, instagram, linkedin, youtube, tiktok, snapchat, whatsapp, telegram
}

// Policy represents a versioned platform policy document (Terms, Privacy, Refund, etc.).
type Policy struct {
	ID          int64      `json:"id"`
	PolicyKey   string     `json:"policy_key"`
	Version     string     `json:"version"`
	Title       i18n.Text  `json:"title"`
	Content     i18n.Text  `json:"content"`
	Summary     i18n.Text  `json:"summary"`
	IsPublished bool       `json:"is_published"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedBy   *int64     `json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// SQLLog records an executed SQL query from the Developer SQL Console.
type SQLLog struct {
	ID           int64     `json:"id"`
	Query        string    `json:"query"`
	ExecutedBy   *int64    `json:"executed_by,omitempty"`
	ActorName    string    `json:"actor_name,omitempty"`
	DurationMS   int64     `json:"duration_ms"`
	RowsAffected int64     `json:"rows_affected"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// SQLQueryResult represents the tabular output of a SQL query execution.
type SQLQueryResult struct {
	Columns      []string `json:"columns"`
	Rows         [][]any  `json:"rows"`
	DurationMS   int64    `json:"duration_ms"`
	RowsAffected int64    `json:"rows_affected"`
	Error        string   `json:"error,omitempty"`
	Truncated    bool     `json:"truncated,omitempty"`
	Message      string   `json:"message,omitempty"`
}

// AIRoleModel represents an operator-configured model assignment for a platform AI role.
type AIRoleModel struct {
	Role      string    `json:"role"`
	Model     string    `json:"model"`
	IsActive  bool      `json:"is_active"`
	MaxTokens *int      `json:"max_tokens,omitempty"`
	Notes     string    `json:"notes,omitempty"`
	UpdatedBy *int64    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}
