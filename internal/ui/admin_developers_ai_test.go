package ui

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

type mockAIDevRepo struct {
	platformadmin.Repository
	settings   *platformadmin.GatewaySettings
	aiSettings *platformadmin.AISettings
	roleModels map[string]*platformadmin.AIRoleModel
}

func newMockAIDevRepo() *mockAIDevRepo {
	return &mockAIDevRepo{
		settings: &platformadmin.GatewaySettings{
			EndpointURL: "https://api.muhiya.com",
			IsActive:    true,
			FastModel:   "qwen3.7-flash",
		},
		aiSettings: &platformadmin.AISettings{
			EndpointURL: "https://api.muhiya.com",
			IsActive:    true,
		},
		roleModels: make(map[string]*platformadmin.AIRoleModel),
	}
}

func (m *mockAIDevRepo) GetSetting(_ context.Context, key string) (*platformadmin.SystemSetting, error) {
	if key == "gateway_configuration" {
		return &platformadmin.SystemSetting{
			Key: key,
			Value: map[string]any{
				"endpoint_url": "https://api.muhiya.com",
				"is_active":    true,
				"fast_model":   "qwen3.7-flash",
			},
		}, nil
	}
	if key == "ai_configuration" {
		return &platformadmin.SystemSetting{
			Key: key,
			Value: map[string]any{
				"endpoint_url": "https://api.muhiya.com",
				"is_active":    true,
			},
		}, nil
	}
	return nil, nil
}

func (m *mockAIDevRepo) SetSetting(_ context.Context, s *platformadmin.SystemSetting) error {
	return nil
}

func (m *mockAIDevRepo) ListSQLLogs(_ context.Context, _, _ int) ([]*platformadmin.SQLLog, error) {
	return nil, nil
}
func (m *mockAIDevRepo) GetGatewaySettings(_ context.Context) (*platformadmin.GatewaySettings, error) {
	return m.settings, nil
}
func (m *mockAIDevRepo) SaveGatewaySettings(_ context.Context, s *platformadmin.GatewaySettings) error {
	m.settings = s
	return nil
}
func (m *mockAIDevRepo) GetAISettings(_ context.Context) (*platformadmin.AISettings, error) {
	return m.aiSettings, nil
}
func (m *mockAIDevRepo) SaveAISettings(_ context.Context, s *platformadmin.AISettings) error {
	m.aiSettings = s
	return nil
}
func (m *mockAIDevRepo) ListAIRoleModels(_ context.Context) ([]*platformadmin.AIRoleModel, error) {
	res := make([]*platformadmin.AIRoleModel, 0, len(m.roleModels))
	for _, rm := range m.roleModels {
		res = append(res, rm)
	}
	return res, nil
}
func (m *mockAIDevRepo) GetAIRoleModel(_ context.Context, role string) (*platformadmin.AIRoleModel, error) {
	rm, ok := m.roleModels[role]
	if !ok {
		return nil, nil
	}
	return rm, nil
}
func (m *mockAIDevRepo) SaveAIRoleModel(_ context.Context, rm *platformadmin.AIRoleModel) error {
	rm.UpdatedAt = time.Now()
	m.roleModels[rm.Role] = rm
	return nil
}
func (m *mockAIDevRepo) DeleteAIRoleModel(_ context.Context, role string) error {
	delete(m.roleModels, role)
	return nil
}
func (m *mockAIDevRepo) ListErrorLogs(_ context.Context, _ platformadmin.ErrorLogFilter) ([]*platformadmin.ErrorLog, int, error) {
	return nil, 0, nil
}
func (m *mockAIDevRepo) GetErrorDiagnosticsMetrics(_ context.Context) (int, int, int, int, error) {
	return 0, 0, 0, 0, nil
}
func (m *mockAIDevRepo) ListAuditLogWithFilter(_ context.Context, _ platformadmin.AuditLogFilter) ([]*platformadmin.AuditEntry, int, error) {
	return nil, 0, nil
}
func (m *mockAIDevRepo) ListSEOPages(_ context.Context, _ platformadmin.SEOPagesFilter) ([]*platformadmin.SEOPage, int, error) {
	return nil, 0, nil
}
func (m *mockAIDevRepo) GetSEOPageByRoute(_ context.Context, _ string) (*platformadmin.SEOPage, error) {
	return nil, nil
}
func (m *mockAIDevRepo) UpsertSEOPage(_ context.Context, _ *platformadmin.SEOPage) error {
	return nil
}
func (m *mockAIDevRepo) GetSEOSettings(_ context.Context) (*platformadmin.SEOSettings, error) {
	return nil, nil
}
func (m *mockAIDevRepo) UpdateSEORobotsTxt(_ context.Context, _ string) error {
	return nil
}

func setupAIDevTestHandler() (*UIHandler, *mockAIDevRepo) {
	repo := newMockAIDevRepo()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	adminSvc := platformadmin.NewService(repo, logger)
	handler := &UIHandler{log: logger, adminSvc: adminSvc}
	return handler, repo
}

func TestAdminAIFetchModelsAPI(t *testing.T) {
	h, _ := setupAIDevTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/admin/developers/ai/fetch-models", nil)
	rec := httptest.NewRecorder()

	h.AdminAIFetchModelsAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var res struct {
		Models []ModelOption `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(res.Models) == 0 {
		t.Fatalf("expected non-empty models list")
	}

	foundQwen := false
	for _, m := range res.Models {
		if m.ID == "qwen3.7-flash" {
			foundQwen = true
			break
		}
	}
	if !foundQwen {
		t.Errorf("expected qwen3.7-flash in model options")
	}
}

func TestAdminAISaveRoleModelSubmit(t *testing.T) {
	h, repo := setupAIDevTestHandler()

	// 1. Submit via form
	formData := url.Values{
		"role":       {"matching.vendor_import"},
		"model":      {"custom-model-test"},
		"is_active":  {"true"},
		"max_tokens": {"2048"},
		"notes":      {"test notes"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/developers/ai/role-models", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	actor := authctx.Actor{UserID: 42, OrgID: 1}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	h.AdminAISaveRoleModelSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}

	saved, err := repo.GetAIRoleModel(context.Background(), "matching.vendor_import")
	if err != nil || saved == nil {
		t.Fatalf("expected role model to be saved, got %v", err)
	}
	if saved.Model != "custom-model-test" {
		t.Errorf("expected custom-model-test, got %s", saved.Model)
	}
	if !saved.IsActive {
		t.Errorf("expected is_active to be true")
	}
	if saved.MaxTokens == nil || *saved.MaxTokens != 2048 {
		t.Errorf("expected max_tokens 2048, got %v", saved.MaxTokens)
	}
	if saved.UpdatedBy == nil || *saved.UpdatedBy != 42 {
		t.Errorf("expected updated_by 42, got %v", saved.UpdatedBy)
	}

	// 2. Submit via JSON Accept header
	formData2 := url.Values{
		"role":      {"matching.smart_order"},
		"model":     {"qwen-smart-order"},
		"is_active": {"true"},
	}
	req2 := httptest.NewRequest(http.MethodPost, "/admin/developers/ai/role-models", strings.NewReader(formData2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Accept", "application/json")
	rec2 := httptest.NewRecorder()

	h.AdminAISaveRoleModelSubmit(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for JSON request, got %d", rec2.Code)
	}

	var jsonResp map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &jsonResp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if jsonResp["status"] != "ok" || jsonResp["role"] != "matching.smart_order" {
		t.Errorf("unexpected json response: %+v", jsonResp)
	}
}

func TestAdminDevelopersPageAITab(t *testing.T) {
	h, repo := setupAIDevTestHandler()

	// Seed custom role model
	tokens := 1024
	_ = repo.SaveAIRoleModel(context.Background(), &platformadmin.AIRoleModel{
		Role:      "matching.vendor_import",
		Model:     "custom-vendor-qwen",
		IsActive:  true,
		MaxTokens: &tokens,
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/developers?tab=ai", nil)
	actor := authctx.Actor{UserID: 1, Role: "admin"}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	h.AdminDevelopersPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify the 4 mandatory sections exist
	if !strings.Contains(body, "1. الاتصال ببوابة الذكاء الاصطناعي") {
		t.Errorf("missing section 1: connection")
	}
	if !strings.Contains(body, "2. النماذج والأدوار") {
		t.Errorf("missing section 2: models table")
	}
	if !strings.Contains(body, "3. الحدود وسقوف الأمان") {
		t.Errorf("missing section 3: ceilings")
	}
	if !strings.Contains(body, "4. الاستهلاك ومراقبة التكاليف") {
		t.Errorf("missing section 4: consumption logs link")
	}

	// Verify all 4 matching tools are present in the table
	if !strings.Contains(body, "مطابقة استيراد المورد") {
		t.Errorf("missing tool: مطابقة استيراد المورد")
	}
	if !strings.Contains(body, "مطابقة منتجات التوفير") {
		t.Errorf("missing tool: مطابقة منتجات التوفير")
	}
	if !strings.Contains(body, "الطلب الذكي بالذكاء الاصطناعي") {
		t.Errorf("missing tool: الطلب الذكي بالذكاء الاصطناعي")
	}
	if !strings.Contains(body, "استيراد دليل الإدارة") {
		t.Errorf("missing tool: استيراد دليل الإدارة")
	}

	// Verify the custom saved model is rendered
	if !strings.Contains(body, "custom-vendor-qwen") {
		t.Errorf("expected custom saved model 'custom-vendor-qwen' to be rendered")
	}
}
