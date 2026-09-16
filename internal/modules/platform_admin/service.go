package platformadmin

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Service manages system configurations and geographical data.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a platform admin service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// GetSetting retrieves a configuration setting.
func (s *Service) GetSetting(ctx context.Context, key string) (*SystemSetting, error) {
	return s.setting(ctx, key)
}

// SetSetting writes a configuration setting.
func (s *Service) SetSetting(ctx context.Context, setting *SystemSetting) error {
	if err := setting.Validate(); err != nil {
		return err
	}
	if err := s.storeSetting(ctx, setting); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "system setting updated", "key", setting.Key)
	return nil
}

// ListPublicSettings returns client-facing settings.
func (s *Service) ListPublicSettings(ctx context.Context) ([]*SystemSetting, error) {
	return s.repo.ListPublicSettings(ctx)
}

// ListCountries returns supported countries.
func (s *Service) ListCountries(ctx context.Context) ([]*Country, error) {
	return s.repo.ListCountries(ctx)
}

// ListGovernorates returns active governorates for a country.
func (s *Service) ListGovernorates(ctx context.Context, countryID int64) ([]*Governorate, error) {
	return s.repo.ListGovernorates(ctx, countryID)
}

// ListAllGovernorates returns all governorates for admin management.
func (s *Service) ListAllGovernorates(ctx context.Context, countryID int64) ([]*Governorate, error) {
	return s.repo.ListAllGovernorates(ctx, countryID)
}

// GetGovernorate returns a single governorate by ID.
func (s *Service) GetGovernorate(ctx context.Context, id int64) (*Governorate, error) {
	return s.repo.GetGovernorate(ctx, id)
}

// CreateGovernorate adds a new governorate.
func (s *Service) CreateGovernorate(ctx context.Context, g *Governorate) error {
	return s.repo.CreateGovernorate(ctx, g)
}

// UpdateGovernorate updates an existing governorate.
func (s *Service) UpdateGovernorate(ctx context.Context, g *Governorate) error {
	return s.repo.UpdateGovernorate(ctx, g)
}

// ToggleGovernorateStatus toggles the active status of a governorate.
func (s *Service) ToggleGovernorateStatus(ctx context.Context, id int64) error {
	return s.repo.ToggleGovernorateStatus(ctx, id)
}

// ListCities returns active cities for a country.
func (s *Service) ListCities(ctx context.Context, countryID int64) ([]*City, error) {
	return s.repo.ListCities(ctx, countryID)
}

// ListAllCities returns all cities (both active and inactive) for admin management.
func (s *Service) ListAllCities(ctx context.Context, countryID int64) ([]*City, error) {
	return s.repo.ListAllCities(ctx, countryID)
}

// ListCitiesByGovernorate returns all cities belonging to a specific governorate.
func (s *Service) ListCitiesByGovernorate(ctx context.Context, governorateID int64) ([]*City, error) {
	return s.repo.ListCitiesByGovernorate(ctx, governorateID)
}

// GetCity returns a single city by ID.
func (s *Service) GetCity(ctx context.Context, id int64) (*City, error) {
	return s.repo.GetCity(ctx, id)
}

// ToggleCityStatus toggles the active state of a city.
func (s *Service) ToggleCityStatus(ctx context.Context, id int64) error {
	return s.repo.ToggleCityStatus(ctx, id)
}

// CreateCity adds a new city with spatial coordinates.
func (s *Service) CreateCity(ctx context.Context, c *City) error {
	return s.repo.CreateCity(ctx, c)
}

// UpdateCity updates an existing city with new data.
func (s *Service) UpdateCity(ctx context.Context, c *City) error {
	return s.repo.UpdateCity(ctx, c)
}

// ListCurrencies returns supported currencies.
func (s *Service) ListCurrencies(ctx context.Context) ([]*Currency, error) {
	return s.repo.ListCurrencies(ctx)
}

// ListLanguages returns supported UI languages.
func (s *Service) ListLanguages(ctx context.Context) ([]*Language, error) {
	return s.repo.ListLanguages(ctx)
}

// SubmitContactMessage records a contact inquiry.
func (s *Service) SubmitContactMessage(ctx context.Context, m *ContactMessage) error {
	m.Status = "unread"
	return s.repo.CreateContactMessage(ctx, m)
}

// ListContactMessages returns contact inquiries.
func (s *Service) ListContactMessages(ctx context.Context, status string, limit, offset int) ([]*ContactMessage, error) {
	return s.repo.ListContactMessages(ctx, status, limit, offset)
}

// ListContactMessagesWithTotal returns contact inquiries with total count.
func (s *Service) ListContactMessagesWithTotal(ctx context.Context, status string, limit, offset int) ([]*ContactMessage, int, error) {
	return s.repo.ListContactMessagesWithTotal(ctx, status, limit, offset)
}

// UpdateContactMessageStatus updates the read/in-progress status of a contact message.
func (s *Service) UpdateContactMessageStatus(ctx context.Context, id int64, status string) error {
	return s.repo.UpdateContactMessageStatus(ctx, id, status)
}

// DeleteContactMessage removes a contact message.
func (s *Service) DeleteContactMessage(ctx context.Context, id int64) error {
	return s.repo.DeleteContactMessage(ctx, id)
}

// ListContentBlocks returns all CMS content blocks.
func (s *Service) ListContentBlocks(ctx context.Context) ([]*ContentBlock, error) {
	return s.repo.ListContentBlocks(ctx)
}

// ListContentBlocksWithTotal returns paginated CMS content blocks with total count.
func (s *Service) ListContentBlocksWithTotal(ctx context.Context, limit, offset int) ([]*ContentBlock, int, error) {
	return s.repo.ListContentBlocksWithTotal(ctx, limit, offset)
}

// GetContentBlockByKey returns one CMS block by key.
func (s *Service) GetContentBlockByKey(ctx context.Context, key string) (*ContentBlock, error) {
	return s.repo.GetContentBlockByKey(ctx, key)
}

// UpsertContentBlock creates or updates a CMS block.
func (s *Service) UpsertContentBlock(ctx context.Context, b *ContentBlock) error {
	if b.Key == "" {
		return apperr.Validation("content.key_required", "A content block key is required.", nil)
	}
	return s.repo.UpsertContentBlock(ctx, b)
}

// ToggleContentBlockStatus toggles is_active for a content block.
func (s *Service) ToggleContentBlockStatus(ctx context.Context, id int64) error {
	return s.repo.ToggleContentBlockStatus(ctx, id)
}

// DeleteContentBlock removes a content block by ID.
func (s *Service) DeleteContentBlock(ctx context.Context, id int64) error {
	return s.repo.DeleteContentBlock(ctx, id)
}

// RecordVisitor records one visitor-session-day, deduplicated by key+day.
func (s *Service) RecordVisitor(ctx context.Context, v *Visitor) error {
	if v.VisitorKey == "" {
		return nil
	}
	return s.repo.RecordVisitor(ctx, v)
}

// VisitorAnalytics returns the aggregate traffic view for the admin page.
func (s *Service) VisitorAnalytics(ctx context.Context, limit int) (*VisitorAnalytics, error) {
	return s.repo.VisitorAnalytics(ctx, limit)
}

// VisitorAnalyticsWithTotal returns the aggregate traffic view with pagination.
func (s *Service) VisitorAnalyticsWithTotal(ctx context.Context, limit, offset int) (*VisitorAnalytics, error) {
	return s.repo.VisitorAnalyticsWithTotal(ctx, limit, offset)
}

// ListAuditLog returns the platform audit trail, newest first.
func (s *Service) ListAuditLog(ctx context.Context, limit, offset int) ([]*AuditEntry, error) {
	return s.repo.ListAuditLog(ctx, limit, offset)
}

// QueueStats returns River job counts grouped by state.
func (s *Service) QueueStats(ctx context.Context) (map[string]int, error) {
	return s.repo.QueueStats(ctx)
}

// ListPolicyVersions returns all historical and current versions of a policy.
func (s *Service) ListPolicyVersions(ctx context.Context, policyKey string) ([]*Policy, error) {
	return s.repo.ListPolicyVersions(ctx, policyKey)
}

// GetPolicyVersion retrieves a policy by key and version string.
func (s *Service) GetPolicyVersion(ctx context.Context, policyKey, version string) (*Policy, error) {
	return s.repo.GetPolicyVersion(ctx, policyKey, version)
}

// GetActivePolicy retrieves the current live published policy for public rendering.
func (s *Service) GetActivePolicy(ctx context.Context, policyKey string) (*Policy, error) {
	return s.repo.GetActivePolicy(ctx, policyKey)
}

// CreatePolicyVersion saves a new policy draft version.
func (s *Service) CreatePolicyVersion(ctx context.Context, p *Policy) error {
	if p.PolicyKey == "" {
		return apperr.Validation("policy.key_required", "Policy key is required.", nil)
	}
	if p.Version == "" {
		p.Version = "1.0"
	}
	if err := s.repo.CreatePolicyVersion(ctx, p); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "policy version created", "key", p.PolicyKey, "version", p.Version)
	return nil
}

// PublishPolicyVersion makes a specific version the live active document.
func (s *Service) PublishPolicyVersion(ctx context.Context, id int64) error {
	if err := s.repo.PublishPolicyVersion(ctx, id); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "policy version published", "id", id)
	return nil
}

// GetAISettings loads AI configuration from database settings.
func (s *Service) GetAISettings(ctx context.Context) (*AISettings, error) {
	setting, err := s.setting(ctx, "ai_configuration")
	if err != nil || setting == nil || setting.Value == nil {
		return &AISettings{
			Temperature:  0.7,
			MaxTokens:    2048,
			SystemPrompt: i18n.TDefault("w4_mod.24_237"),
			IsActive:     true,
		}, nil
	}
	v := setting.Value
	ai := &AISettings{
		APIKey:       getString(v, "api_key", ""),
		EndpointURL:  getString(v, "endpoint_url", "https://api.muhiya.com"),
		Temperature:  getFloat(v, "temperature", 0.7),
		MaxTokens:    getInt(v, "max_tokens", 2048),
		SystemPrompt: getString(v, "system_prompt", ""),
		IsActive:     getBool(v, "is_active", true),
	}
	return ai, nil
}

// SaveAISettings writes AI configuration to database settings.
func (s *Service) SaveAISettings(ctx context.Context, ai *AISettings) error {
	val := map[string]any{
		"api_key":       ai.APIKey,
		"endpoint_url":  ai.EndpointURL,
		"temperature":   ai.Temperature,
		"max_tokens":    ai.MaxTokens,
		"system_prompt": ai.SystemPrompt,
		"is_active":     ai.IsActive,
	}
	return s.storeSetting(ctx, &SystemSetting{
		Key:         "ai_configuration",
		Value:       val,
		Description: "Platform AI Configuration",
		IsPublic:    false,
	})
}

// GetGatewaySettings loads API Gateway settings from database.
func (s *Service) GetGatewaySettings(ctx context.Context) (*GatewaySettings, error) {
	setting, err := s.setting(ctx, "gateway_configuration")
	if err != nil || setting == nil || setting.Value == nil {
		return &GatewaySettings{
			EndpointURL:    "https://api.dawa24.com/v1",
			Environment:    "production",
			TimeoutSeconds: 30,
			IsActive:       true,
		}, nil
	}
	v := setting.Value
	gw := &GatewaySettings{
		EndpointURL:    getString(v, "endpoint_url", "https://api.dawa24.com/v1"),
		Environment:    getString(v, "environment", "production"),
		TimeoutSeconds: getInt(v, "timeout_seconds", 30),
		APIKey:         getString(v, "api_key", ""),
		IsActive:       getBool(v, "is_active", true),
		VirtualKey:     getString(v, "virtual_key", ""),
		AIUserID:       getString(v, "ai_user_id", ""),
		FastModel:      getString(v, "fast_model", ""),
		QualityModel:   getString(v, "quality_model", ""),
		AIPlanID:       getString(v, "ai_plan_id", ""),
	}
	if gw.FastModel == "nemotron-3.5-lightning" {
		gw.FastModel = "qwen3.7-flash"
	}
	if gw.QualityModel == "nemotron-3.5-lightning" {
		gw.QualityModel = "qwen3.7-flash"
	}
	return gw, nil
}

// SaveGatewaySettings writes API Gateway settings to database.
//
// The administrator credential is validated before it is stored, because this
// value is sent as Basic auth to an external host on every management call.
// The live configuration was found holding the production database password;
// see gateway_credential.go.
func (s *Service) SaveGatewaySettings(ctx context.Context, gw *GatewaySettings) error {
	if err := ValidateAdminCredential(gw.APIKey); err != nil {
		return err
	}
	val := map[string]any{
		"endpoint_url":    gw.EndpointURL,
		"environment":     gw.Environment,
		"timeout_seconds": gw.TimeoutSeconds,
		"api_key":         gw.APIKey,
		"is_active":       gw.IsActive,
		"virtual_key":     gw.VirtualKey,
		"ai_user_id":      gw.AIUserID,
		"fast_model":      gw.FastModel,
		"quality_model":   gw.QualityModel,
		"ai_plan_id":      gw.AIPlanID,
	}
	return s.storeSetting(ctx, &SystemSetting{
		Key:         "gateway_configuration",
		Value:       val,
		Description: "Platform API Gateway Endpoints Configuration",
		IsPublic:    false,
	})
}

// ListAIRoleModels returns all configured role-to-model mappings.
func (s *Service) ListAIRoleModels(ctx context.Context) ([]*AIRoleModel, error) {
	return s.repo.ListAIRoleModels(ctx)
}

// GetAIRoleModel retrieves a specific role-to-model configuration.
func (s *Service) GetAIRoleModel(ctx context.Context, role string) (*AIRoleModel, error) {
	return s.repo.GetAIRoleModel(ctx, role)
}

// SaveAIRoleModel stores or updates a role-to-model configuration.
func (s *Service) SaveAIRoleModel(ctx context.Context, rm *AIRoleModel) error {
	if rm == nil || rm.Role == "" || rm.Model == "" {
		return apperr.Validation("ai.invalid_role_model", "role and model are required", nil)
	}
	if err := s.repo.SaveAIRoleModel(ctx, rm); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "ai role model configured", "role", rm.Role, "model", rm.Model, "active", rm.IsActive)
	return nil
}

// DeleteAIRoleModel removes an AI role model mapping.
func (s *Service) DeleteAIRoleModel(ctx context.Context, role string) error {
	return s.repo.DeleteAIRoleModel(ctx, role)
}
