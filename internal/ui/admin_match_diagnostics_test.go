package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockDiagnosticsMatchingSvc struct {
	run  *matching.MatchRun
	rows []matching.DiagnosticRowData
}

func (m *mockDiagnosticsMatchingSvc) Prepare(context.Context, matching.PrepareRequest) (matching.PrepareResult, error) {
	return matching.PrepareResult{}, nil
}
func (m *mockDiagnosticsMatchingSvc) GetRun(_ context.Context, id int64) (*matching.MatchRun, error) {
	if m.run != nil && m.run.ID == id {
		return m.run, nil
	}
	return nil, nil
}
func (m *mockDiagnosticsMatchingSvc) ListRunRows(_ context.Context, _ matching.Scope, _ int64, _ int) ([]matching.DiagnosticRowData, error) {
	return m.rows, nil
}
func (m *mockDiagnosticsMatchingSvc) Diagnose(_ context.Context, _ matching.Scope, _ int64, _ productmatch.Row, dec matchflow.RowDecision) (matching.DiagnosticRow, error) {
	return matching.DiagnosticRow{
		RunID:          m.run.ID,
		OrganizationID: m.run.OrganizationID,
		State:          m.run.State,
		Decision:       dec,
	}, nil
}
func (m *mockDiagnosticsMatchingSvc) MarkReviewable(context.Context, int64) error { return nil }
func (m *mockDiagnosticsMatchingSvc) Rematch(context.Context, matching.Scope, productmatch.Row, int) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}
func (m *mockDiagnosticsMatchingSvc) Search(context.Context, matching.Scope, string, productmatch.Row, int) ([]matchflow.CandidateView, error) {
	return nil, nil
}
func (m *mockDiagnosticsMatchingSvc) Review(context.Context, matching.Scope, matching.ReviewAction) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}
func (m *mockDiagnosticsMatchingSvc) PreviewBulk(context.Context, matching.Scope, matching.ReviewAction, []matching.RowRef) (matching.BulkPreview, error) {
	return matching.BulkPreview{}, nil
}
func (m *mockDiagnosticsMatchingSvc) StartEnhancement(context.Context, int64, []matching.EnhanceRow) (matching.EnhanceHandle, error) {
	return matching.EnhanceHandle{}, nil
}
func (m *mockDiagnosticsMatchingSvc) StopEnhancement(context.Context, int64) error { return nil }
func (m *mockDiagnosticsMatchingSvc) Revalidate(context.Context, matching.Scope, matching.RowRef, matchflow.RowDecision) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}
func (m *mockDiagnosticsMatchingSvc) Remember(context.Context, matching.Scope, matching.KnowledgeWrite) error {
	return nil
}
func (m *mockDiagnosticsMatchingSvc) ListKnowledge(context.Context, matching.Scope, matching.KnowledgeFilter) (matching.Page[matching.Knowledge], error) {
	return matching.Page[matching.Knowledge]{}, nil
}
func (m *mockDiagnosticsMatchingSvc) RevokeKnowledge(context.Context, matching.Scope, int64) error {
	return nil
}
func (m *mockDiagnosticsMatchingSvc) PromoteKnowledge(context.Context, matching.Scope, int64) error {
	return nil
}
func (m *mockDiagnosticsMatchingSvc) RegisterApplier(matching.Workflow, matching.Applier) {}
func (m *mockDiagnosticsMatchingSvc) GetApplier(matching.Workflow) matching.Applier       { return nil }
func (m *mockDiagnosticsMatchingSvc) ListAIConfigurations(context.Context) ([]matching.AIConfiguration, error) {
	return nil, nil
}
func (m *mockDiagnosticsMatchingSvc) GetAIConfiguration(context.Context, string) (*matching.AIConfiguration, error) {
	return nil, nil
}
func (m *mockDiagnosticsMatchingSvc) UpdateAIConfigurationStatus(context.Context, string, string, string, string) error {
	return nil
}
func (m *mockDiagnosticsMatchingSvc) ListPolicies(context.Context) ([]matching.PolicyRecord, error) {
	return nil, nil
}
func (m *mockDiagnosticsMatchingSvc) ListAssignments(context.Context) ([]matching.PolicyAssignment, error) {
	return nil, nil
}
func (m *mockDiagnosticsMatchingSvc) UpdateAssignment(context.Context, string, string, string, *string, string, string) error {
	return nil
}
func (m *mockDiagnosticsMatchingSvc) RollbackAssignment(context.Context, string, string, string) error {
	return nil
}



func setupDiagnosticsHandler(matchingMock matching.Service) *UIHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &mockCatalogMatchingRepo{mockCatalogImageRepo: newMockCatalogImageRepo()}
	catSvc := catalog.NewService(mockRepo, logger)
	catSvc.SetMatchingService(matchingMock)
	return &UIHandler{log: logger, catSvc: catSvc}
}

func TestAdminMatchDiagnostics_CrossTenantForbidden(t *testing.T) {
	run := &matching.MatchRun{
		ID:             101,
		OrganizationID: 10,
		Workflow:       "vendor",
		State:          "reviewable",
		CreatedAt:      time.Now(),
		HeartbeatAt:    time.Now(),
	}
	h := setupDiagnosticsHandler(&mockDiagnosticsMatchingSvc{run: run})

	r := chi.NewRouter()
	r.Get("/admin/matching/runs/{id}/diagnose", h.AdminMatchDiagnosticsPage)

	// Actor from different org (org 20), not staff, not admin
	req := httptest.NewRequest(http.MethodGet, "/admin/matching/runs/101/diagnose", nil)
	ctx := authctx.WithActor(req.Context(), authctx.Actor{
		UserID:         5,
		OrganizationID: 20,
		IsStaff:        false,
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAdminMatchDiagnostics_StaffAllowed(t *testing.T) {
	run := &matching.MatchRun{
		ID:             101,
		OrganizationID: 10,
		Workflow:       "vendor",
		State:          "reviewable",
		CreatedAt:      time.Now(),
		HeartbeatAt:    time.Now(),
	}
	h := setupDiagnosticsHandler(&mockDiagnosticsMatchingSvc{run: run})

	r := chi.NewRouter()
	r.Get("/admin/matching/runs/{id}/diagnose", h.AdminMatchDiagnosticsPage)

	// Actor from org 20, but IsStaff = true
	req := httptest.NewRequest(http.MethodGet, "/admin/matching/runs/101/diagnose", nil)
	ctx := authctx.WithActor(req.Context(), authctx.Actor{
		UserID:         1,
		OrganizationID: 20,
		IsStaff:        true,
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "تشخيص المطابقة")
}

func TestAdminMatchDiagnostics_SameTenantAllowed(t *testing.T) {
	run := &matching.MatchRun{
		ID:             101,
		OrganizationID: 10,
		Workflow:       "vendor",
		State:          "reviewable",
		CreatedAt:      time.Now(),
		HeartbeatAt:    time.Now(),
	}
	h := setupDiagnosticsHandler(&mockDiagnosticsMatchingSvc{run: run})

	r := chi.NewRouter()
	r.Get("/admin/matching/runs/{id}/diagnose", h.AdminMatchDiagnosticsPage)

	// Actor from same org (org 10)
	req := httptest.NewRequest(http.MethodGet, "/admin/matching/runs/101/diagnose", nil)
	ctx := authctx.WithActor(req.Context(), authctx.Actor{
		UserID:         2,
		OrganizationID: 10,
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAdminMatchDiagnostics_RunNotFound(t *testing.T) {
	h := setupDiagnosticsHandler(&mockDiagnosticsMatchingSvc{})

	r := chi.NewRouter()
	r.Get("/admin/matching/runs/{id}/diagnose", h.AdminMatchDiagnosticsPage)

	req := httptest.NewRequest(http.MethodGet, "/admin/matching/runs/999/diagnose", nil)
	ctx := authctx.WithActor(req.Context(), authctx.Actor{
		UserID:  1,
		IsStaff: true,
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Run not found")
}
