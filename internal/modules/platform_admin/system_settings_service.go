package platformadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"net/http"
	"strings"
	"time"
)

// IsDecisionMemoryEnabled checks if the platform-wide decision memory system is active.
func (s *Service) IsDecisionMemoryEnabled(ctx context.Context) (bool, error) {
	setting, err := s.setting(ctx, "decision_memory_enabled")
	if err != nil || setting == nil || setting.Value == nil {
		return true, nil // default enabled
	}
	return getBool(setting.Value, "enabled", true), nil
}

// SetDecisionMemoryEnabled updates the global decision memory active state.
func (s *Service) SetDecisionMemoryEnabled(ctx context.Context, enabled bool) error {
	return s.storeSetting(ctx, &SystemSetting{
		Key:         "decision_memory_enabled",
		Value:       map[string]any{"enabled": enabled},
		Description: "Global switch to enable or disable AI Decision Memory across all platform features",
		IsPublic:    true,
	})
}

// GetSiteSettings loads public website branding, contact info, and social links.
func (s *Service) GetSiteSettings(ctx context.Context) (*SiteSettings, error) {
	setting, err := s.setting(ctx, "site_public_settings")
	if err != nil || setting == nil || setting.Value == nil {
		return &SiteSettings{
			SiteName:        i18n.TDefault("w4_ui.24_28"),
			SiteDescription: i18n.TDefault("w4_ui.s_99_99"),
			LogoURL:         "/static/img/logo.png",
			LogoDarkURL:     "",
			FaviconURL:      "/static/img/favicon.png",
			ContactEmail:    "info@dawa24.com",
			SupportEmail:    "support@dawa24.com",
			Phone:           "01065397000",
			WhatsApp:        "201065397000",
			Address:         i18n.TDefault("w4_ui.s_100_100"),
			SocialLinks: map[string]string{
				"facebook":  "https://facebook.com/dawa24",
				"twitter":   "https://twitter.com/dawa24",
				"instagram": "https://instagram.com/dawa24",
				"linkedin":  "https://linkedin.com/company/dawa24",
				"youtube":   "https://youtube.com/@dawa24",
				"tiktok":    "https://tiktok.com/@dawa24",
				"snapchat":  "https://snapchat.com/add/dawa24",
				"whatsapp":  "https://wa.me/201065397000",
				"telegram":  "https://t.me/dawa24",
			},
		}, nil
	}
	v := setting.Value
	socialMap := map[string]string{}
	if rawSocial, ok := v["social_links"].(map[string]any); ok {
		for k, val := range rawSocial {
			socialMap[k] = fmt.Sprintf("%v", val)
		}
	} else if rawSocialStr, ok := v["social_links"].(map[string]string); ok {
		socialMap = rawSocialStr
	}

	idleTimeoutMins := getInt(v, "session_idle_timeout_minutes", 30)
	if idleTimeoutMins <= 0 {
		idleTimeoutMins = 30
	}

	ss := &SiteSettings{
		SiteName:                  getString(v, "site_name", i18n.TDefault("w4_ui.24_28")),
		SiteDescription:           getString(v, "site_description", i18n.TDefault("w4_mod.s_424_424")),
		LogoURL:                   getString(v, "logo_url", "/static/img/logo.png"),
		LogoDarkURL:               getString(v, "logo_dark_url", ""),
		FaviconURL:                getString(v, "favicon_url", "/static/img/favicon.png"),
		ContactEmail:              getString(v, "contact_email", "info@dawa24.com"),
		SupportEmail:              getString(v, "support_email", "support@dawa24.com"),
		Phone:                     getString(v, "phone", "01065397000"),
		WhatsApp:                  getString(v, "whatsapp", "201065397000"),
		Address:                   getString(v, "address", i18n.TDefault("w4_ui.s_100_100")),
		SessionIdleTimeoutMinutes: idleTimeoutMins,
		SocialLinks:               socialMap,
	}
	return ss, nil
}

// SaveSiteSettings writes public site settings and branding to database.
func (s *Service) SaveSiteSettings(ctx context.Context, ss *SiteSettings) error {
	idleMins := ss.SessionIdleTimeoutMinutes
	if idleMins <= 0 {
		idleMins = 30
	}
	val := map[string]any{
		"site_name":                    ss.SiteName,
		"site_description":             ss.SiteDescription,
		"logo_url":                     ss.LogoURL,
		"logo_dark_url":                ss.LogoDarkURL,
		"favicon_url":                  ss.FaviconURL,
		"contact_email":                ss.ContactEmail,
		"support_email":                ss.SupportEmail,
		"phone":                        ss.Phone,
		"whatsapp":                     ss.WhatsApp,
		"address":                      ss.Address,
		"session_idle_timeout_minutes": idleMins,
		"social_links":                 ss.SocialLinks,
	}
	return s.storeSetting(ctx, &SystemSetting{
		Key:         "site_public_settings",
		Value:       val,
		Description: "Public website branding, contact info, and social media",
		IsPublic:    true,
	})
}

func getString(m map[string]any, k, def string) string {
	if val, ok := m[k]; ok {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func getFloat(m map[string]any, k string, def float64) float64 {
	if val, ok := m[k]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return def
}

func getInt(m map[string]any, k string, def int) int {
	if val, ok := m[k]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case int64:
			return int(v)
		}
	}
	return def
}

func getBool(m map[string]any, k string, def bool) bool {
	if val, ok := m[k]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return def
}

// ExecuteSQL runs an arbitrary SQL query against PostgreSQL with duration and logging.
func (s *Service) ExecuteSQL(ctx context.Context, actorID *int64, actorName, query string) (*SQLQueryResult, error) {
	return s.repo.ExecuteSQL(ctx, actorID, actorName, query)
}

// ListSQLLogs returns previous executed queries from the SQL Console.
func (s *Service) ListSQLLogs(ctx context.Context, limit, offset int) ([]*SQLLog, error) {
	return s.repo.ListSQLLogs(ctx, limit, offset)
}

// LogError records a diagnostic error or exception.
func (s *Service) LogError(ctx context.Context, entry *ErrorLog) error {
	return s.repo.LogError(ctx, entry)
}

// ListErrorLogs searches and retrieves diagnostic error logs.
func (s *Service) ListErrorLogs(ctx context.Context, filter ErrorLogFilter) ([]*ErrorLog, int, error) {
	return s.repo.ListErrorLogs(ctx, filter)
}

// GetErrorLogByID returns an individual diagnostic error.
func (s *Service) GetErrorLogByID(ctx context.Context, id int64) (*ErrorLog, error) {
	return s.repo.GetErrorLogByID(ctx, id)
}

// UpdateErrorLogStatus updates error status (NEW, INVESTIGATING, RESOLVED, IGNORED).
func (s *Service) UpdateErrorLogStatus(ctx context.Context, id int64, status string) error {
	return s.repo.UpdateErrorLogStatus(ctx, id, status)
}

// GetErrorDiagnosticsMetrics returns aggregate error metrics for developer dashboard.
func (s *Service) GetErrorDiagnosticsMetrics(ctx context.Context) (total, critical24h, unresolved, affectedUsers int, err error) {
	return s.repo.GetErrorDiagnosticsMetrics(ctx)
}

// FetchGatewayModels dynamically connects to the AI Gateway endpoint to list available LLM models.
func (s *Service) FetchGatewayModels(ctx context.Context, endpointURL, apiKey string) ([]string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(endpointURL), "/")
	if endpoint == "" {
		endpoint = "https://api.muhiya.com"
	}

	reqURL := endpoint + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.log.WarnContext(ctx, "failed to query ai gateway models endpoint", "url", reqURL, "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gateway returned status %d", resp.StatusCode)
	}

	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []string `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range body.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	if len(models) == 0 && len(body.Models) > 0 {
		models = body.Models
	}

	return models, nil
}

// ListAuditLogByOrg returns audit trail entries filtered to an organization.
func (s *Service) ListAuditLogByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*AuditEntry, error) {
	return s.repo.ListAuditLogByOrg(ctx, orgID, limit, offset)
}

// ListAuditLogByOrgWithTotal returns paginated audit trail entries filtered to an organization with total count.
func (s *Service) ListAuditLogByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*AuditEntry, int, error) {
	return s.repo.ListAuditLogByOrgWithTotal(ctx, orgID, limit, offset)
}

// ListOrgStaffAuditLogWithTotal returns paginated audit trail entries strictly performed by organization employees/members.
func (s *Service) ListOrgStaffAuditLogWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*AuditEntry, int, error) {
	return s.repo.ListOrgStaffAuditLogWithTotal(ctx, orgID, limit, offset)
}

// ListAuditLogWithFilter returns audit trail entries according to the given filter.
func (s *Service) ListAuditLogWithFilter(ctx context.Context, filter AuditLogFilter) ([]*AuditEntry, int, error) {
	return s.repo.ListAuditLogWithFilter(ctx, filter)
}

const SettingTempWarehouseLifecycle = "platform.temp_warehouse_lifecycle"

// GetTempWarehouseLifecycleSettings retrieves the auto-archive and retention configuration for temporary warehouses.
func (s *Service) GetTempWarehouseLifecycleSettings(ctx context.Context) (*TempWarehouseLifecycleSettings, error) {
	setting, err := s.setting(ctx, SettingTempWarehouseLifecycle)
	if err != nil || setting == nil || setting.Value == nil {
		return &TempWarehouseLifecycleSettings{
			AutoArchiveHours:   720, // 30 days default
			AutoArchiveEnabled: true,
			AutoDeleteDays:     30, // 30 days retention after archiving
			AutoDeleteEnabled:  true,
		}, nil
	}

	v := setting.Value
	hours := getInt(v, "auto_archive_hours", 720)
	if hours <= 0 {
		hours = 720
	}
	days := getInt(v, "auto_delete_days", 30)
	if days <= 0 {
		days = 30
	}

	return &TempWarehouseLifecycleSettings{
		AutoArchiveHours:   hours,
		AutoArchiveEnabled: getBool(v, "auto_archive_enabled", true),
		AutoDeleteDays:     days,
		AutoDeleteEnabled:  getBool(v, "auto_delete_enabled", true),
	}, nil
}

// SaveTempWarehouseLifecycleSettings persists the auto-archive and retention configuration.
func (s *Service) SaveTempWarehouseLifecycleSettings(ctx context.Context, cfg *TempWarehouseLifecycleSettings) error {
	if cfg == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	if cfg.AutoArchiveHours <= 0 {
		cfg.AutoArchiveHours = 720
	}
	if cfg.AutoDeleteDays <= 0 {
		cfg.AutoDeleteDays = 30
	}

	return s.storeSetting(ctx, &SystemSetting{
		Key: SettingTempWarehouseLifecycle,
		Value: map[string]any{
			"auto_archive_hours":   cfg.AutoArchiveHours,
			"auto_archive_enabled": cfg.AutoArchiveEnabled,
			"auto_delete_days":     cfg.AutoDeleteDays,
			"auto_delete_enabled":  cfg.AutoDeleteEnabled,
		},
		Description: "Automatic archiving and retention purging rules for temporary warehouses",
		IsPublic:    false,
	})
}

// AuditEntryBackend reads a single audit entry with its diff.
type AuditEntryBackend interface {
	GetAuditEntryByID(ctx context.Context, id int64) (*AuditEntry, error)
}

// GetAuditEntryByID returns one audit entry, or nil when it does not exist.
func (s *Service) GetAuditEntryByID(ctx context.Context, id int64) (*AuditEntry, error) {
	backend, ok := s.repo.(AuditEntryBackend)
	if !ok {
		return nil, nil
	}
	return backend.GetAuditEntryByID(ctx, id)
}
