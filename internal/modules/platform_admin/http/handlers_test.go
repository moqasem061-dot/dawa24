package http_test

import (
	"context"
	"testing"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) GetSetting(context.Context, string) (*platformadmin.SystemSetting, error) {
	r.fail("GetSetting")
	return nil, nil
}
func (r stubRepo) SetSetting(context.Context, *platformadmin.SystemSetting) error {
	r.fail("SetSetting")
	return nil
}
func (r stubRepo) ListPublicSettings(context.Context) ([]*platformadmin.SystemSetting, error) {
	r.fail("ListPublicSettings")
	return nil, nil
}
func (r stubRepo) ListCountries(context.Context) ([]*platformadmin.Country, error) {
	r.fail("ListCountries")
	return nil, nil
}
func (r stubRepo) ListGovernorates(context.Context, int64) ([]*platformadmin.Governorate, error) {
	r.fail("ListGovernorates")
	return nil, nil
}
func (r stubRepo) ListAllGovernorates(context.Context, int64) ([]*platformadmin.Governorate, error) {
	r.fail("ListAllGovernorates")
	return nil, nil
}
func (r stubRepo) GetGovernorate(context.Context, int64) (*platformadmin.Governorate, error) {
	r.fail("GetGovernorate")
	return nil, nil
}
func (r stubRepo) CreateGovernorate(context.Context, *platformadmin.Governorate) error {
	r.fail("CreateGovernorate")
	return nil
}
func (r stubRepo) UpdateGovernorate(context.Context, *platformadmin.Governorate) error {
	r.fail("UpdateGovernorate")
	return nil
}
func (r stubRepo) ToggleGovernorateStatus(context.Context, int64) error {
	r.fail("ToggleGovernorateStatus")
	return nil
}
func (r stubRepo) ListCities(context.Context, int64) ([]*platformadmin.City, error) {
	r.fail("ListCities")
	return nil, nil
}
func (r stubRepo) ListAllCities(context.Context, int64) ([]*platformadmin.City, error) {
	r.fail("ListAllCities")
	return nil, nil
}
func (r stubRepo) ListCitiesByGovernorate(context.Context, int64) ([]*platformadmin.City, error) {
	r.fail("ListCitiesByGovernorate")
	return nil, nil
}
func (r stubRepo) GetCity(context.Context, int64) (*platformadmin.City, error) {
	r.fail("GetCity")
	return nil, nil
}
func (r stubRepo) ToggleCityStatus(context.Context, int64) error {
	r.fail("ToggleCityStatus")
	return nil
}
func (r stubRepo) CreateCity(context.Context, *platformadmin.City) error {
	r.fail("CreateCity")
	return nil
}
func (r stubRepo) UpdateCity(context.Context, *platformadmin.City) error {
	r.fail("UpdateCity")
	return nil
}
func (r stubRepo) ListCurrencies(context.Context) ([]*platformadmin.Currency, error) {
	r.fail("ListCurrencies")
	return nil, nil
}
func (r stubRepo) ListLanguages(context.Context) ([]*platformadmin.Language, error) {
	r.fail("ListLanguages")
	return nil, nil
}
func (r stubRepo) CreateContactMessage(context.Context, *platformadmin.ContactMessage) error {
	r.fail("CreateContactMessage")
	return nil
}
func (r stubRepo) ListContactMessages(context.Context, string, int, int) ([]*platformadmin.ContactMessage, error) {
	r.fail("ListContactMessages")
	return nil, nil
}
func (r stubRepo) ListContactMessagesWithTotal(context.Context, string, int, int) ([]*platformadmin.ContactMessage, int, error) {
	r.fail("ListContactMessagesWithTotal")
	return nil, 0, nil
}
func (r stubRepo) UpdateContactMessageStatus(context.Context, int64, string) error {
	r.fail("UpdateContactMessageStatus")
	return nil
}
func (r stubRepo) DeleteContactMessage(context.Context, int64) error {
	r.fail("DeleteContactMessage")
	return nil
}

func (r stubRepo) ListContentBlocks(context.Context) ([]*platformadmin.ContentBlock, error) {
	r.fail("ListContentBlocks")
	return nil, nil
}
func (r stubRepo) ListContentBlocksWithTotal(context.Context, int, int) ([]*platformadmin.ContentBlock, int, error) {
	r.fail("ListContentBlocksWithTotal")
	return nil, 0, nil
}

func (r stubRepo) GetContentBlockByKey(context.Context, string) (*platformadmin.ContentBlock, error) {
	r.fail("GetContentBlockByKey")
	return nil, nil
}

func (r stubRepo) UpsertContentBlock(context.Context, *platformadmin.ContentBlock) error {
	r.fail("UpsertContentBlock")
	return nil
}
func (r stubRepo) ToggleContentBlockStatus(context.Context, int64) error {
	r.fail("ToggleContentBlockStatus")
	return nil
}
func (r stubRepo) DeleteContentBlock(context.Context, int64) error {
	r.fail("DeleteContentBlock")
	return nil
}

func (r stubRepo) RecordVisitor(context.Context, *platformadmin.Visitor) error {
	r.fail("RecordVisitor")
	return nil
}

func (r stubRepo) VisitorAnalytics(context.Context, int) (*platformadmin.VisitorAnalytics, error) {
	r.fail("VisitorAnalytics")
	return nil, nil
}
func (r stubRepo) VisitorAnalyticsWithTotal(context.Context, int, int) (*platformadmin.VisitorAnalytics, error) {
	r.fail("VisitorAnalyticsWithTotal")
	return nil, nil
}

func (r stubRepo) ListAuditLog(context.Context, int, int) ([]*platformadmin.AuditEntry, error) {
	r.fail("ListAuditLog")
	return nil, nil
}
func (r stubRepo) ListAuditLogByOrg(context.Context, int64, int, int) ([]*platformadmin.AuditEntry, error) {
	r.fail("ListAuditLogByOrg")
	return nil, nil
}
func (r stubRepo) ListAuditLogByOrgWithTotal(context.Context, int64, int, int) ([]*platformadmin.AuditEntry, int, error) {
	r.fail("ListAuditLogByOrgWithTotal")
	return nil, 0, nil
}
func (r stubRepo) ListOrgStaffAuditLogWithTotal(context.Context, int64, int, int) ([]*platformadmin.AuditEntry, int, error) {
	r.fail("ListOrgStaffAuditLogWithTotal")
	return nil, 0, nil
}
func (r stubRepo) ListAuditLogWithFilter(context.Context, platformadmin.AuditLogFilter) ([]*platformadmin.AuditEntry, int, error) {
	r.fail("ListAuditLogWithFilter")
	return nil, 0, nil
}

func (r stubRepo) ListPolicyVersions(ctx context.Context, key string) ([]*platformadmin.Policy, error) {
	r.fail("ListPolicyVersions")
	return nil, nil
}
func (r stubRepo) GetPolicyVersion(ctx context.Context, key, version string) (*platformadmin.Policy, error) {
	r.fail("GetPolicyVersion")
	return nil, nil
}
func (r stubRepo) GetActivePolicy(ctx context.Context, key string) (*platformadmin.Policy, error) {
	r.fail("GetActivePolicy")
	return nil, nil
}
func (r stubRepo) CreatePolicyVersion(ctx context.Context, p *platformadmin.Policy) error {
	r.fail("CreatePolicyVersion")
	return nil
}
func (r stubRepo) PublishPolicyVersion(ctx context.Context, id int64) error {
	r.fail("PublishPolicyVersion")
	return nil
}

func (r stubRepo) ExecuteSQL(context.Context, *int64, string, string) (*platformadmin.SQLQueryResult, error) {
	r.fail("ExecuteSQL")
	return nil, nil
}
func (r stubRepo) ListSQLLogs(context.Context, int, int) ([]*platformadmin.SQLLog, error) {
	r.fail("ListSQLLogs")
	return nil, nil
}
func (r stubRepo) LogError(context.Context, *platformadmin.ErrorLog) error {
	r.fail("LogError")
	return nil
}
func (r stubRepo) ListErrorLogs(context.Context, platformadmin.ErrorLogFilter) ([]*platformadmin.ErrorLog, int, error) {
	r.fail("ListErrorLogs")
	return nil, 0, nil
}
func (r stubRepo) GetErrorLogByID(context.Context, int64) (*platformadmin.ErrorLog, error) {
	r.fail("GetErrorLogByID")
	return nil, nil
}
func (r stubRepo) UpdateErrorLogStatus(context.Context, int64, string) error {
	r.fail("UpdateErrorLogStatus")
	return nil
}
func (r stubRepo) GetErrorDiagnosticsMetrics(context.Context) (int, int, int, int, error) {
	r.fail("GetErrorDiagnosticsMetrics")
	return 0, 0, 0, 0, nil
}

func (r stubRepo) QueueStats(context.Context) (map[string]int, error) {
	r.fail("QueueStats")
	return nil, nil
}

func (r stubRepo) ListTranslations(context.Context, platformadmin.TranslationFilter) ([]*platformadmin.Translation, int, error) {
	r.fail("ListTranslations")
	return nil, 0, nil
}
func (r stubRepo) GetTranslationByKey(context.Context, string) (*platformadmin.Translation, error) {
	r.fail("GetTranslationByKey")
	return nil, nil
}
func (r stubRepo) UpsertTranslation(context.Context, *platformadmin.Translation) error {
	r.fail("UpsertTranslation")
	return nil
}
func (r stubRepo) DeleteTranslation(context.Context, string) error {
	r.fail("DeleteTranslation")
	return nil
}
func (r stubRepo) GetTranslationStats(context.Context) (*platformadmin.TranslationStats, error) {
	r.fail("GetTranslationStats")
	return nil, nil
}
func (r stubRepo) LoadAllCustomTranslations(context.Context) (map[string]i18n.Text, error) {
	r.fail("LoadAllCustomTranslations")
	return nil, nil
}

type happyRepo struct{}

func (happyRepo) GetSetting(ctx context.Context, key string) (*platformadmin.SystemSetting, error) {
	return &platformadmin.SystemSetting{Key: key, Value: map[string]any{"mode": "dark"}}, nil
}
func (happyRepo) SetSetting(ctx context.Context, s *platformadmin.SystemSetting) error {
	return nil
}
func (happyRepo) ListPublicSettings(ctx context.Context) ([]*platformadmin.SystemSetting, error) {
	return []*platformadmin.SystemSetting{{Key: "site_name", Value: map[string]any{"name": "Dawa24"}}}, nil
}
func (happyRepo) ListCountries(ctx context.Context) ([]*platformadmin.Country, error) {
	return []*platformadmin.Country{{ID: 1, Name: i18n.Text{"en": "Egypt"}, Code: "EG"}}, nil
}
func (happyRepo) ListGovernorates(ctx context.Context, countryID int64) ([]*platformadmin.Governorate, error) {
	return []*platformadmin.Governorate{{ID: 1, CountryID: countryID, Name: i18n.Text{"en": "Cairo"}, IsActive: true}}, nil
}
func (happyRepo) ListAllGovernorates(ctx context.Context, countryID int64) ([]*platformadmin.Governorate, error) {
	return []*platformadmin.Governorate{{ID: 1, CountryID: countryID, Name: i18n.Text{"en": "Cairo"}, IsActive: true}}, nil
}
func (happyRepo) GetGovernorate(ctx context.Context, id int64) (*platformadmin.Governorate, error) {
	return &platformadmin.Governorate{ID: id, CountryID: 1, Name: i18n.Text{"en": "Cairo"}, IsActive: true}, nil
}
func (happyRepo) CreateGovernorate(ctx context.Context, g *platformadmin.Governorate) error {
	return nil
}
func (happyRepo) UpdateGovernorate(ctx context.Context, g *platformadmin.Governorate) error {
	return nil
}
func (happyRepo) ToggleGovernorateStatus(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) ListCities(ctx context.Context, countryID int64) ([]*platformadmin.City, error) {
	return []*platformadmin.City{{ID: 1, CountryID: countryID, Name: i18n.Text{"en": "Cairo"}, IsActive: true}}, nil
}
func (happyRepo) ListAllCities(ctx context.Context, countryID int64) ([]*platformadmin.City, error) {
	return []*platformadmin.City{{ID: 1, CountryID: countryID, Name: i18n.Text{"en": "Cairo"}, IsActive: true}}, nil
}
func (happyRepo) ListCitiesByGovernorate(ctx context.Context, governorateID int64) ([]*platformadmin.City, error) {
	return []*platformadmin.City{{ID: 1, CountryID: 1, Name: i18n.Text{"en": "Cairo"}, IsActive: true}}, nil
}
func (happyRepo) GetCity(ctx context.Context, id int64) (*platformadmin.City, error) {
	return &platformadmin.City{ID: id, CountryID: 1, Name: i18n.Text{"en": "Cairo"}, IsActive: true}, nil
}
func (happyRepo) ToggleCityStatus(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) CreateCity(ctx context.Context, c *platformadmin.City) error {
	return nil
}
func (happyRepo) UpdateCity(ctx context.Context, c *platformadmin.City) error {
	return nil
}
func (happyRepo) ListCurrencies(ctx context.Context) ([]*platformadmin.Currency, error) {
	return []*platformadmin.Currency{{ID: 1, Code: "EGP", Name: i18n.Text{"en": "Egyptian Pound"}}}, nil
}
func (happyRepo) ListLanguages(ctx context.Context) ([]*platformadmin.Language, error) {
	return []*platformadmin.Language{{ID: 1, Code: "ar", Name: "Arabic"}}, nil
}
func (happyRepo) CreateContactMessage(ctx context.Context, m *platformadmin.ContactMessage) error {
	m.ID = 1
	return nil
}
func (happyRepo) ListContactMessages(ctx context.Context, status string, limit, offset int) ([]*platformadmin.ContactMessage, error) {
	return []*platformadmin.ContactMessage{{ID: 1, Name: "User", Email: "u@example.com", Message: "Help"}}, nil
}
func (happyRepo) ListContactMessagesWithTotal(ctx context.Context, status string, limit, offset int) ([]*platformadmin.ContactMessage, int, error) {
	return []*platformadmin.ContactMessage{{ID: 1, Name: "User", Email: "u@example.com", Message: "Help"}}, 1, nil
}
func (happyRepo) UpdateContactMessageStatus(ctx context.Context, id int64, status string) error {
	return nil
}
func (happyRepo) DeleteContactMessage(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) MarkContactMessageRead(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) ListPolicyVersions(ctx context.Context, key string) ([]*platformadmin.Policy, error) {
	return nil, nil
}
func (happyRepo) GetPolicyVersion(ctx context.Context, key, version string) (*platformadmin.Policy, error) {
	return nil, nil
}
func (happyRepo) GetActivePolicy(ctx context.Context, key string) (*platformadmin.Policy, error) {
	return nil, nil
}
func (happyRepo) CreatePolicyVersion(ctx context.Context, p *platformadmin.Policy) error {
	return nil
}
func (happyRepo) PublishPolicyVersion(ctx context.Context, id int64) error {
	return nil
}

func (happyRepo) ExecuteSQL(ctx context.Context, actorID *int64, actorName, query string) (*platformadmin.SQLQueryResult, error) {
	return &platformadmin.SQLQueryResult{Columns: []string{"col1"}, Rows: [][]any{{"val1"}}}, nil
}
func (happyRepo) ListSQLLogs(ctx context.Context, limit, offset int) ([]*platformadmin.SQLLog, error) {
	return nil, nil
}
func (happyRepo) LogError(ctx context.Context, entry *platformadmin.ErrorLog) error {
	return nil
}

func (r stubRepo) ListAIRoleModels(context.Context) ([]*platformadmin.AIRoleModel, error) {
	r.fail("ListAIRoleModels")
	return nil, nil
}
func (r stubRepo) GetAIRoleModel(context.Context, string) (*platformadmin.AIRoleModel, error) {
	r.fail("GetAIRoleModel")
	return nil, nil
}
func (r stubRepo) SaveAIRoleModel(context.Context, *platformadmin.AIRoleModel) error {
	r.fail("SaveAIRoleModel")
	return nil
}
func (r stubRepo) DeleteAIRoleModel(context.Context, string) error {
	r.fail("DeleteAIRoleModel")
	return nil
}

func (happyRepo) ListAIRoleModels(context.Context) ([]*platformadmin.AIRoleModel, error) {
	return nil, nil
}
func (happyRepo) GetAIRoleModel(context.Context, string) (*platformadmin.AIRoleModel, error) {
	return nil, nil
}
func (happyRepo) SaveAIRoleModel(context.Context, *platformadmin.AIRoleModel) error {
	return nil
}
func (happyRepo) DeleteAIRoleModel(context.Context, string) error {
	return nil
}
