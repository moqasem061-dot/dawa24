package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockPolicyMatchingSvc struct {
	mockDiagnosticsMatchingSvc
	policies        []matching.PolicyRecord
	assignments     []matching.PolicyAssignment
	updatedWorkflow string
	updatedCohort   string
	updatedActive   string
	updatedShadow   *string
	updatedBy       string
	updatedReason   string
	rollbackWF      string
	rollbackCohort  string
	rollbackBy      string
}

func (m *mockPolicyMatchingSvc) ListPolicies(context.Context) ([]matching.PolicyRecord, error) {
	return m.policies, nil
}

func (m *mockPolicyMatchingSvc) ListAssignments(context.Context) ([]matching.PolicyAssignment, error) {
	return m.assignments, nil
}

func (m *mockPolicyMatchingSvc) UpdateAssignment(_ context.Context, workflow, cohort, activeVersion string, shadowVersion *string, changedBy, reason string) error {
	m.updatedWorkflow = workflow
	m.updatedCohort = cohort
	m.updatedActive = activeVersion
	m.updatedShadow = shadowVersion
	m.updatedBy = changedBy
	m.updatedReason = reason
	return nil
}

func (m *mockPolicyMatchingSvc) RollbackAssignment(_ context.Context, workflow, cohort, changedBy string) error {
	m.rollbackWF = workflow
	m.rollbackCohort = cohort
	m.rollbackBy = changedBy
	return nil
}

func TestAdminMatchPoliciesPage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &mockCatalogMatchingRepo{mockCatalogImageRepo: newMockCatalogImageRepo()}
	catSvc := catalog.NewService(mockRepo, logger)

	shadowVer := "pm-shadow-v2"
	matchingMock := &mockPolicyMatchingSvc{
		policies: []matching.PolicyRecord{
			{
				Version:   "pm-legacy-1",
				Body:      productmatch.DefaultPolicy(),
				CreatedBy: "system",
				CreatedAt: time.Now(),
			},
			{
				Version:   "pm-shadow-v2",
				Body:      productmatch.DefaultPolicy(),
				CreatedBy: "admin@dawa24.com",
				CreatedAt: time.Now(),
			},
		},
		assignments: []matching.PolicyAssignment{
			{
				Workflow:      "vendor",
				Cohort:        "*",
				ActiveVersion: "pm-legacy-1",
				ShadowVersion: &shadowVer,
				ChangedBy:     "admin@dawa24.com",
				ChangedAt:     time.Now(),
			},
		},
	}
	catSvc.SetMatchingService(matchingMock)
	h := &UIHandler{log: logger, catSvc: catSvc}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admin/matching/policies", nil)
	rec := httptest.NewRecorder()

	h.AdminMatchPoliciesPage(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "pm-legacy-1")
	assert.Contains(t, body, "pm-shadow-v2")
	assert.Contains(t, body, "vendor")
}

func TestAdminMatchPolicyUpdateAssignment(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &mockCatalogMatchingRepo{mockCatalogImageRepo: newMockCatalogImageRepo()}
	catSvc := catalog.NewService(mockRepo, logger)

	matchingMock := &mockPolicyMatchingSvc{}
	catSvc.SetMatchingService(matchingMock)
	h := &UIHandler{log: logger, catSvc: catSvc}

	form := url.Values{}
	form.Set("workflow", "vendor")
	form.Set("cohort", "*")
	form.Set("active_version", "pm-legacy-1")
	form.Set("shadow_version", "pm-shadow-v2")
	form.Set("reason", "Rollout candidate shadow test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/matching/policies/assign", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	actor := authctx.Actor{
		UserID: 42,
		Email:  "reviewer@dawa24.com",
	}
	ctx := authctx.WithActor(req.Context(), actor)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.AdminMatchPolicyUpdateAssignment(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "vendor", matchingMock.updatedWorkflow)
	assert.Equal(t, "*", matchingMock.updatedCohort)
	assert.Equal(t, "pm-legacy-1", matchingMock.updatedActive)
	assert.NotNil(t, matchingMock.updatedShadow)
	assert.Equal(t, "pm-shadow-v2", *matchingMock.updatedShadow)
	assert.Equal(t, "reviewer@dawa24.com", matchingMock.updatedBy)
	assert.Equal(t, "Rollout candidate shadow test", matchingMock.updatedReason)
}

func TestAdminMatchPolicyRollback(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &mockCatalogMatchingRepo{mockCatalogImageRepo: newMockCatalogImageRepo()}
	catSvc := catalog.NewService(mockRepo, logger)

	matchingMock := &mockPolicyMatchingSvc{}
	catSvc.SetMatchingService(matchingMock)
	h := &UIHandler{log: logger, catSvc: catSvc}

	form := url.Values{}
	form.Set("workflow", "vendor")
	form.Set("cohort", "*")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/matching/policies/rollback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	actor := authctx.Actor{
		UserID: 42,
		Email:  "admin@dawa24.com",
	}
	ctx := authctx.WithActor(req.Context(), actor)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.AdminMatchPolicyRollback(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "vendor", matchingMock.rollbackWF)
	assert.Equal(t, "*", matchingMock.rollbackCohort)
	assert.Equal(t, "admin@dawa24.com", matchingMock.rollbackBy)
}
