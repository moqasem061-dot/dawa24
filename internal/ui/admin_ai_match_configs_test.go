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

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

type mockConfigsMatchingSvc struct {
	mockDiagnosticsMatchingSvc
	configs       []matching.AIConfiguration
	updatedID     string
	updatedStatus string
	updatedBy     string
	updatedReason string
}

func (m *mockConfigsMatchingSvc) ListAIConfigurations(context.Context) ([]matching.AIConfiguration, error) {
	return m.configs, nil
}

func (m *mockConfigsMatchingSvc) GetAIConfiguration(_ context.Context, id string) (*matching.AIConfiguration, error) {
	for _, c := range m.configs {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, nil
}

func (m *mockConfigsMatchingSvc) UpdateAIConfigurationStatus(_ context.Context, id string, newStatus string, changedBy string, reason string) error {
	m.updatedID = id
	m.updatedStatus = newStatus
	m.updatedBy = changedBy
	m.updatedReason = reason
	return nil
}

func TestAdminAIMatchConfigsPage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &mockCatalogMatchingRepo{mockCatalogImageRepo: newMockCatalogImageRepo()}
	catSvc := catalog.NewService(mockRepo, logger)

	cost, _ := money.Parse("0.05")
	matchingMock := &mockConfigsMatchingSvc{
		configs: []matching.AIConfiguration{
			{
				ID:                 "pm-default-v8",
				GatewayRoute:       "product.match.v8",
				PromptVersion:      "sm-enh-v8",
				ContractVersion:    "v8",
				NativeStructured:   true,
				BytesPerToken:      4.0,
				MaxItemsPerAttempt: 20,
				MaxInputTokens:     4000,
				MaxOutputTokens:    1000,
				MaxCostPerAttempt:  cost,
				Status:             "candidate",
				CreatedBy:          "system",
				CreatedAt:          time.Now(),
				UpdatedAt:          time.Now(),
			},
		},
	}
	catSvc.SetMatchingService(matchingMock)
	h := &UIHandler{log: logger, catSvc: catSvc}

	req := httptest.NewRequest(http.MethodGet, "/admin/matching/ai-configs", nil)
	rec := httptest.NewRecorder()

	h.AdminAIMatchConfigsPage(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "pm-default-v8")
	assert.Contains(t, body, "product.match.v8")
	assert.Contains(t, body, "candidate")
}

func TestAdminAIMatchConfigUpdateStatus(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &mockCatalogMatchingRepo{mockCatalogImageRepo: newMockCatalogImageRepo()}
	catSvc := catalog.NewService(mockRepo, logger)

	matchingMock := &mockConfigsMatchingSvc{}
	catSvc.SetMatchingService(matchingMock)
	h := &UIHandler{log: logger, catSvc: catSvc}

	form := url.Values{}
	form.Set("status", "qualified")
	form.Set("reason", "Verified on benchmark test set")

	req := httptest.NewRequest(http.MethodPost, "/admin/matching/ai-configs/pm-default-v8/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Set chi URLParam
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "pm-default-v8")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)

	// Set auth user
	actor := authctx.Actor{
		UserID: 42,
		Email:  "admin@dawa24.com",
	}
	ctx = authctx.WithActor(ctx, actor)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.AdminAIMatchConfigUpdateStatus(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "pm-default-v8", matchingMock.updatedID)
	assert.Equal(t, "qualified", matchingMock.updatedStatus)
	assert.Equal(t, "admin@dawa24.com", matchingMock.updatedBy)
	assert.Equal(t, "Verified on benchmark test set", matchingMock.updatedReason)
}
