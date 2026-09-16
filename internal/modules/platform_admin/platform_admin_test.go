package platformadmin

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type mockPlatformAdminRepo struct {
	settings   map[string]*SystemSetting
	countries  []*Country
	cities     map[int64][]*City
	currencies []*Currency
	languages  []*Language
	messages   []*ContactMessage
}

func newMockPlatformAdminRepo() *mockPlatformAdminRepo {
	return &mockPlatformAdminRepo{
		settings: map[string]*SystemSetting{},
		cities:   map[int64][]*City{},
	}
}

func (m *mockPlatformAdminRepo) GetSetting(_ context.Context, key string) (*SystemSetting, error) {
	s, ok := m.settings[key]
	if !ok {
		return nil, apperr.NotFound("setting")
	}
	return s, nil
}

func (m *mockPlatformAdminRepo) SetSetting(_ context.Context, s *SystemSetting) error {
	m.settings[s.Key] = s
	return nil
}

func (m *mockPlatformAdminRepo) ListPublicSettings(_ context.Context) ([]*SystemSetting, error) {
	var list []*SystemSetting
	for _, s := range m.settings {
		if s.IsPublic {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockPlatformAdminRepo) ListCountries(_ context.Context) ([]*Country, error) {
	return m.countries, nil
}

func (m *mockPlatformAdminRepo) ListGovernorates(_ context.Context, _ int64) ([]*Governorate, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) ListAllGovernorates(_ context.Context, _ int64) ([]*Governorate, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) GetGovernorate(_ context.Context, _ int64) (*Governorate, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) CreateGovernorate(_ context.Context, _ *Governorate) error {
	return nil
}

func (m *mockPlatformAdminRepo) UpdateGovernorate(_ context.Context, _ *Governorate) error {
	return nil
}

func (m *mockPlatformAdminRepo) ToggleGovernorateStatus(_ context.Context, _ int64) error {
	return nil
}

func (m *mockPlatformAdminRepo) ListCities(_ context.Context, countryID int64) ([]*City, error) {
	return m.cities[countryID], nil
}

func (m *mockPlatformAdminRepo) ListAllCities(_ context.Context, countryID int64) ([]*City, error) {
	return m.cities[countryID], nil
}

func (m *mockPlatformAdminRepo) ListCitiesByGovernorate(_ context.Context, _ int64) ([]*City, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) GetCity(_ context.Context, _ int64) (*City, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) ToggleCityStatus(_ context.Context, _ int64) error {
	return nil
}

func (m *mockPlatformAdminRepo) CreateCity(_ context.Context, _ *City) error {
	return nil
}

func (m *mockPlatformAdminRepo) UpdateCity(_ context.Context, _ *City) error {
	return nil
}

func (m *mockPlatformAdminRepo) ListCurrencies(_ context.Context) ([]*Currency, error) {
	return m.currencies, nil
}

func (m *mockPlatformAdminRepo) ListLanguages(_ context.Context) ([]*Language, error) {
	return m.languages, nil
}

func (m *mockPlatformAdminRepo) CreateContactMessage(_ context.Context, msg *ContactMessage) error {
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockPlatformAdminRepo) ListContactMessages(_ context.Context, status string, limit, offset int) ([]*ContactMessage, error) {
	return m.messages, nil
}

func (m *mockPlatformAdminRepo) ListContactMessagesWithTotal(_ context.Context, status string, limit, offset int) ([]*ContactMessage, int, error) {
	return m.messages, len(m.messages), nil
}

func (m *mockPlatformAdminRepo) UpdateContactMessageStatus(_ context.Context, id int64, status string) error {
	for _, msg := range m.messages {
		if msg.ID == id {
			msg.Status = status
		}
	}
	return nil
}

func (m *mockPlatformAdminRepo) DeleteContactMessage(_ context.Context, id int64) error {
	var filtered []*ContactMessage
	for _, msg := range m.messages {
		if msg.ID != id {
			filtered = append(filtered, msg)
		}
	}
	m.messages = filtered
	return nil
}

func (m *mockPlatformAdminRepo) ListContentBlocks(_ context.Context) ([]*ContentBlock, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) ListContentBlocksWithTotal(_ context.Context, _, _ int) ([]*ContentBlock, int, error) {
	return nil, 0, nil
}

func (m *mockPlatformAdminRepo) GetContentBlockByKey(_ context.Context, _ string) (*ContentBlock, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) UpsertContentBlock(_ context.Context, _ *ContentBlock) error {
	return nil
}

func (m *mockPlatformAdminRepo) ToggleContentBlockStatus(_ context.Context, _ int64) error {
	return nil
}

func (m *mockPlatformAdminRepo) DeleteContentBlock(_ context.Context, _ int64) error {
	return nil
}

func (m *mockPlatformAdminRepo) RecordVisitor(_ context.Context, _ *Visitor) error {
	return nil
}

func (m *mockPlatformAdminRepo) VisitorAnalytics(_ context.Context, _ int) (*VisitorAnalytics, error) {
	return &VisitorAnalytics{}, nil
}
func (m *mockPlatformAdminRepo) VisitorAnalyticsWithTotal(_ context.Context, _, _ int) (*VisitorAnalytics, error) {
	return &VisitorAnalytics{}, nil
}

func (m *mockPlatformAdminRepo) ListAuditLog(_ context.Context, _, _ int) ([]*AuditEntry, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) ListAuditLogWithFilter(_ context.Context, _ AuditLogFilter) ([]*AuditEntry, int, error) {
	return nil, 0, nil
}

func (m *mockPlatformAdminRepo) QueueStats(_ context.Context) (map[string]int, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) ListPolicyVersions(_ context.Context, _ string) ([]*Policy, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) GetPolicyVersion(_ context.Context, _, _ string) (*Policy, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) GetActivePolicy(_ context.Context, _ string) (*Policy, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) CreatePolicyVersion(_ context.Context, _ *Policy) error {
	return nil
}

func (m *mockPlatformAdminRepo) PublishPolicyVersion(_ context.Context, _ int64) error {
	return nil
}

func (m *mockPlatformAdminRepo) ExecuteSQL(_ context.Context, _ *int64, _, _ string) (*SQLQueryResult, error) {
	return &SQLQueryResult{Columns: []string{"test"}, Rows: [][]any{{"ok"}}}, nil
}

func (m *mockPlatformAdminRepo) ListSQLLogs(_ context.Context, _, _ int) ([]*SQLLog, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) LogError(_ context.Context, _ *ErrorLog) error {
	return nil
}

func (m *mockPlatformAdminRepo) ListErrorLogs(_ context.Context, _ ErrorLogFilter) ([]*ErrorLog, int, error) {
	return nil, 0, nil
}

func (m *mockPlatformAdminRepo) GetErrorLogByID(_ context.Context, _ int64) (*ErrorLog, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) UpdateErrorLogStatus(_ context.Context, _ int64, _ string) error {
	return nil
}

func (m *mockPlatformAdminRepo) GetErrorDiagnosticsMetrics(_ context.Context) (total, critical24h, unresolved, affectedUsers int, err error) {
	return 0, 0, 0, 0, nil
}

func (m *mockPlatformAdminRepo) ListAuditLogByOrg(_ context.Context, _ int64, _, _ int) ([]*AuditEntry, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) ListAuditLogByOrgWithTotal(_ context.Context, _ int64, _, _ int) ([]*AuditEntry, int, error) {
	return nil, 0, nil
}

func (m *mockPlatformAdminRepo) ListOrgStaffAuditLogWithTotal(_ context.Context, _ int64, _, _ int) ([]*AuditEntry, int, error) {
	return nil, 0, nil
}

func (m *mockPlatformAdminRepo) ListTranslations(_ context.Context, _ TranslationFilter) ([]*Translation, int, error) {
	return nil, 0, nil
}

func (m *mockPlatformAdminRepo) GetTranslationByKey(_ context.Context, _ string) (*Translation, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) UpsertTranslation(_ context.Context, _ *Translation) error {
	return nil
}

func (m *mockPlatformAdminRepo) DeleteTranslation(_ context.Context, _ string) error {
	return nil
}

func (m *mockPlatformAdminRepo) GetTranslationStats(_ context.Context) (*TranslationStats, error) {
	return &TranslationStats{}, nil
}

func (m *mockPlatformAdminRepo) LoadAllCustomTranslations(_ context.Context) (map[string]i18n.Text, error) {
	return map[string]i18n.Text{}, nil
}

func (m *mockPlatformAdminRepo) ListAIRoleModels(_ context.Context) ([]*AIRoleModel, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) GetAIRoleModel(_ context.Context, _ string) (*AIRoleModel, error) {
	return nil, nil
}

func (m *mockPlatformAdminRepo) SaveAIRoleModel(_ context.Context, _ *AIRoleModel) error {
	return nil
}

func (m *mockPlatformAdminRepo) DeleteAIRoleModel(_ context.Context, _ string) error {
	return nil
}

func TestPlatformAdminSettingsAndGeo(t *testing.T) {
	ctx := context.Background()
	repo := newMockPlatformAdminRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Set Setting
	err := svc.SetSetting(ctx, &SystemSetting{
		Key:      "site_title",
		Value:    map[string]any{"ar": "دوا 24", "en": "Dawa 24"},
		IsPublic: true,
	})
	if err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}

	setting, err := svc.GetSetting(ctx, "site_title")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if setting.Key != "site_title" || !setting.IsPublic {
		t.Errorf("unexpected setting: %+v", setting)
	}

	publicSettings, err := svc.ListPublicSettings(ctx)
	if err != nil || len(publicSettings) != 1 {
		t.Fatalf("ListPublicSettings failed: %v", err)
	}

	// 2. Geography Data
	repo.countries = []*Country{
		{ID: 1, Code: "EG", Name: i18n.New("مصر", "Egypt"), PhoneCode: "+20", Currency: "EGP", IsActive: true},
	}
	repo.cities[1] = []*City{
		{ID: 10, CountryID: 1, Name: i18n.New("القاهرة", "Cairo"), IsActive: true},
	}
	repo.currencies = []*Currency{
		{Code: "EGP", Name: i18n.New("جنيه مصري", "Egyptian Pound"), Symbol: "EGP", IsActive: true},
	}
	repo.languages = []*Language{
		{Code: "ar", Name: "العربية", Dir: "rtl", IsActive: true},
	}

	countries, err := svc.ListCountries(ctx)
	if err != nil || len(countries) != 1 {
		t.Fatalf("ListCountries failed: %v", err)
	}

	cities, err := svc.ListCities(ctx, 1)
	if err != nil || len(cities) != 1 {
		t.Fatalf("ListCities failed: %v", err)
	}

	currencies, err := svc.ListCurrencies(ctx)
	if err != nil || len(currencies) != 1 {
		t.Fatalf("ListCurrencies failed: %v", err)
	}

	languages, err := svc.ListLanguages(ctx)
	if err != nil || len(languages) != 1 {
		t.Fatalf("ListLanguages failed: %v", err)
	}

	// 3. Contact Messages
	err = svc.SubmitContactMessage(ctx, &ContactMessage{
		Name:    "Amr",
		Email:   "amr@example.com",
		Message: "Inquiry regarding API access",
	})
	if err != nil {
		t.Fatalf("SubmitContactMessage failed: %v", err)
	}

	msgs, err := svc.ListContactMessages(ctx, "unread", 10, 0)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("ListContactMessages failed: %v", err)
	}
}

// Trash methods: the trash screens are exercised by their own tests; these
// stubs only satisfy the Repository interface.
func (m mockPlatformAdminRepo) ListSoftDeletableTables(context.Context) ([]*TrashModel, error) {
	return nil, nil
}
func (m mockPlatformAdminRepo) ListTrashedRows(context.Context, string, string, int, int) ([]*TrashRow, error) {
	return nil, nil
}
func (m mockPlatformAdminRepo) ListTrashedRowsWithTotal(context.Context, string, string, string, int, int) ([]*TrashRow, int, error) {
	return nil, 0, nil
}
func (m mockPlatformAdminRepo) RestoreTrashedRow(context.Context, string, string, int64, int64) error {
	return nil
}
func (m mockPlatformAdminRepo) PurgeTrashedRow(context.Context, string, string, int64, int64) error {
	return nil
}
