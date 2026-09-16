package platformadmin

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Repository defines storage operations for system settings and locations.
type Repository interface {
	GetSetting(ctx context.Context, key string) (*SystemSetting, error)
	SetSetting(ctx context.Context, s *SystemSetting) error
	ListPublicSettings(ctx context.Context) ([]*SystemSetting, error)

	ListCountries(ctx context.Context) ([]*Country, error)
	ListGovernorates(ctx context.Context, countryID int64) ([]*Governorate, error)
	ListAllGovernorates(ctx context.Context, countryID int64) ([]*Governorate, error)
	GetGovernorate(ctx context.Context, id int64) (*Governorate, error)
	CreateGovernorate(ctx context.Context, g *Governorate) error
	UpdateGovernorate(ctx context.Context, g *Governorate) error
	ToggleGovernorateStatus(ctx context.Context, id int64) error
	ListCities(ctx context.Context, countryID int64) ([]*City, error)
	ListAllCities(ctx context.Context, countryID int64) ([]*City, error)
	ListCitiesByGovernorate(ctx context.Context, governorateID int64) ([]*City, error)
	GetCity(ctx context.Context, id int64) (*City, error)
	ToggleCityStatus(ctx context.Context, id int64) error
	CreateCity(ctx context.Context, c *City) error
	UpdateCity(ctx context.Context, c *City) error

	ListCurrencies(ctx context.Context) ([]*Currency, error)
	ListLanguages(ctx context.Context) ([]*Language, error)
	CreateContactMessage(ctx context.Context, m *ContactMessage) error
	ListContactMessages(ctx context.Context, status string, limit, offset int) ([]*ContactMessage, error)
	ListContactMessagesWithTotal(ctx context.Context, status string, limit, offset int) ([]*ContactMessage, int, error)
	UpdateContactMessageStatus(ctx context.Context, id int64, status string) error
	DeleteContactMessage(ctx context.Context, id int64) error

	ListContentBlocks(ctx context.Context) ([]*ContentBlock, error)
	ListContentBlocksWithTotal(ctx context.Context, limit, offset int) ([]*ContentBlock, int, error)
	GetContentBlockByKey(ctx context.Context, key string) (*ContentBlock, error)
	UpsertContentBlock(ctx context.Context, b *ContentBlock) error
	ToggleContentBlockStatus(ctx context.Context, id int64) error
	DeleteContentBlock(ctx context.Context, id int64) error

	RecordVisitor(ctx context.Context, v *Visitor) error
	VisitorAnalytics(ctx context.Context, limit int) (*VisitorAnalytics, error)
	VisitorAnalyticsWithTotal(ctx context.Context, limit, offset int) (*VisitorAnalytics, error)

	ListAuditLog(ctx context.Context, limit, offset int) ([]*AuditEntry, error)
	ListAuditLogByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*AuditEntry, error)
	ListAuditLogByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*AuditEntry, int, error)
	ListOrgStaffAuditLogWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*AuditEntry, int, error)
	ListAuditLogWithFilter(ctx context.Context, filter AuditLogFilter) ([]*AuditEntry, int, error)
	QueueStats(ctx context.Context) (map[string]int, error)

	ListPolicyVersions(ctx context.Context, policyKey string) ([]*Policy, error)
	GetPolicyVersion(ctx context.Context, policyKey, version string) (*Policy, error)
	GetActivePolicy(ctx context.Context, policyKey string) (*Policy, error)
	CreatePolicyVersion(ctx context.Context, p *Policy) error
	PublishPolicyVersion(ctx context.Context, id int64) error

	ExecuteSQL(ctx context.Context, actorID *int64, actorName, query string) (*SQLQueryResult, error)
	// Soft-delete recovery (PLAN_V7 Task 2.5). The model list is discovered
	// from information_schema rather than hand-maintained.
	ListSoftDeletableTables(ctx context.Context) ([]*TrashModel, error)
	ListTrashedRows(ctx context.Context, schema, table string, limit, offset int) ([]*TrashRow, error)
	ListTrashedRowsWithTotal(ctx context.Context, schema, table, search string, limit, offset int) ([]*TrashRow, int, error)
	RestoreTrashedRow(ctx context.Context, schema, table string, id, actorID int64) error
	PurgeTrashedRow(ctx context.Context, schema, table string, id, actorID int64) error

	ListSQLLogs(ctx context.Context, limit, offset int) ([]*SQLLog, error)

	LogError(ctx context.Context, entry *ErrorLog) error
	ListErrorLogs(ctx context.Context, filter ErrorLogFilter) ([]*ErrorLog, int, error)
	GetErrorLogByID(ctx context.Context, id int64) (*ErrorLog, error)
	UpdateErrorLogStatus(ctx context.Context, id int64, status string) error
	GetErrorDiagnosticsMetrics(ctx context.Context) (total, critical24h, unresolved, affectedUsers int, err error)

	ListTranslations(ctx context.Context, filter TranslationFilter) ([]*Translation, int, error)
	GetTranslationByKey(ctx context.Context, key string) (*Translation, error)
	UpsertTranslation(ctx context.Context, t *Translation) error
	DeleteTranslation(ctx context.Context, key string) error
	GetTranslationStats(ctx context.Context) (*TranslationStats, error)
	LoadAllCustomTranslations(ctx context.Context) (map[string]i18n.Text, error)

	ListAIRoleModels(ctx context.Context) ([]*AIRoleModel, error)
	GetAIRoleModel(ctx context.Context, role string) (*AIRoleModel, error)
	SaveAIRoleModel(ctx context.Context, rm *AIRoleModel) error
	DeleteAIRoleModel(ctx context.Context, role string) error

	ListSEOPages(ctx context.Context, filter SEOPagesFilter) ([]*SEOPage, int, error)
	GetSEOPageByRoute(ctx context.Context, route string) (*SEOPage, error)
	UpsertSEOPage(ctx context.Context, page *SEOPage) error
	GetSEOSettings(ctx context.Context) (*SEOSettings, error)
	UpdateSEORobotsTxt(ctx context.Context, robotsTxt string) error
}
