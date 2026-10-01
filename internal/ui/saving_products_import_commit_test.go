package ui

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockRevalidateMatchingSvc struct {
	revalidateFn func(ctx context.Context, s matching.Scope, ref matching.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error)
}

func (m *mockRevalidateMatchingSvc) Prepare(_ context.Context, _ matching.PrepareRequest) (matching.PrepareResult, error) {
	return matching.PrepareResult{}, nil
}
func (m *mockRevalidateMatchingSvc) GetRun(_ context.Context, _ int64) (*matching.MatchRun, error) {
	return nil, nil
}
func (m *mockRevalidateMatchingSvc) ListRunRows(_ context.Context, _ matching.Scope, _ int64, _ int) ([]matching.DiagnosticRowData, error) {
	return nil, nil
}
func (m *mockRevalidateMatchingSvc) Diagnose(_ context.Context, _ matching.Scope, _ int64, _ productmatch.Row, _ matchflow.RowDecision) (matching.DiagnosticRow, error) {
	return matching.DiagnosticRow{}, nil
}
func (m *mockRevalidateMatchingSvc) MarkReviewable(_ context.Context, _ int64) error { return nil }
func (m *mockRevalidateMatchingSvc) Rematch(_ context.Context, _ matching.Scope, _ productmatch.Row, _ int) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}
func (m *mockRevalidateMatchingSvc) Search(_ context.Context, _ matching.Scope, _ string, _ productmatch.Row, _ int) ([]matchflow.CandidateView, error) {
	return nil, nil
}
func (m *mockRevalidateMatchingSvc) Review(_ context.Context, _ matching.Scope, _ matching.ReviewAction) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}
func (m *mockRevalidateMatchingSvc) PreviewBulk(_ context.Context, _ matching.Scope, _ matching.ReviewAction, _ []matching.RowRef) (matching.BulkPreview, error) {
	return matching.BulkPreview{}, nil
}
func (m *mockRevalidateMatchingSvc) StartEnhancement(_ context.Context, _ int64, _ []matching.EnhanceRow) (matching.EnhanceHandle, error) {
	return matching.EnhanceHandle{}, nil
}
func (m *mockRevalidateMatchingSvc) StopEnhancement(_ context.Context, _ int64) error { return nil }
func (m *mockRevalidateMatchingSvc) Revalidate(ctx context.Context, s matching.Scope, ref matching.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
	if m.revalidateFn != nil {
		return m.revalidateFn(ctx, s, ref, d)
	}
	return d, nil
}
func (m *mockRevalidateMatchingSvc) Remember(_ context.Context, _ matching.Scope, _ matching.KnowledgeWrite) error {
	return nil
}
func (m *mockRevalidateMatchingSvc) ListKnowledge(_ context.Context, _ matching.Scope, _ matching.KnowledgeFilter) (matching.Page[matching.Knowledge], error) {
	return matching.Page[matching.Knowledge]{}, nil
}
func (m *mockRevalidateMatchingSvc) RevokeKnowledge(_ context.Context, _ matching.Scope, _ int64) error {
	return nil
}
func (m *mockRevalidateMatchingSvc) PromoteKnowledge(_ context.Context, _ matching.Scope, _ int64) error {
	return nil
}
func (m *mockRevalidateMatchingSvc) RegisterApplier(matching.Workflow, matching.Applier) {}
func (m *mockRevalidateMatchingSvc) GetApplier(matching.Workflow) matching.Applier       { return nil }
func (m *mockRevalidateMatchingSvc) ListAIConfigurations(context.Context) ([]matching.AIConfiguration, error) {
	return nil, nil
}
func (m *mockRevalidateMatchingSvc) GetAIConfiguration(context.Context, string) (*matching.AIConfiguration, error) {
	return nil, nil
}
func (m *mockRevalidateMatchingSvc) UpdateAIConfigurationStatus(context.Context, string, string, string, string) error {
	return nil
}
func (m *mockRevalidateMatchingSvc) ListPolicies(context.Context) ([]matching.PolicyRecord, error) {
	return nil, nil
}
func (m *mockRevalidateMatchingSvc) ListAssignments(context.Context) ([]matching.PolicyAssignment, error) {
	return nil, nil
}
func (m *mockRevalidateMatchingSvc) UpdateAssignment(context.Context, string, string, string, *string, string, string) error {
	return nil
}
func (m *mockRevalidateMatchingSvc) RollbackAssignment(context.Context, string, string, string) error {
	return nil
}



func TestRevalidateSavingItems_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const orgID int64 = 88
	const userID int64 = 42

	pid := int64(201)
	price, _ := money.Parse("15.50")

	mockRepo := &mockCatalogMatchingRepo{
		mockCatalogImageRepo: newMockCatalogImageRepo(),
	}
	catSvc := catalog.NewService(mockRepo, logger)

	matchingMock := &mockRevalidateMatchingSvc{
		revalidateFn: func(_ context.Context, s matching.Scope, _ matching.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
			assert.Equal(t, orgID, s.EffectiveOrg)
			return d, nil
		},
	}
	catSvc.SetMatchingService(matchingMock)

	items := []*StagedSavingItem{
		{
			Index:             0,
			NameProduct:       "بانادول إكسترا",
			SKU:               "PAN-24",
			Quantity:          10,
			Price:             price,
			ProductID:         &pid,
			MasterProductName: "بانادول إكسترا 24 قرص",
			Included:          true,
		},
	}

	valid, rejected, err := RevalidateSavingItems(context.Background(), orgID, userID, items, catSvc)
	require.NoError(t, err)
	assert.Equal(t, 0, rejected)
	require.Len(t, valid, 1)
	assert.Equal(t, orgID, valid[0].OrganizationID)
	assert.Equal(t, &pid, valid[0].ProductID)
}

func TestRevalidateSavingItems_CatalogChanged_Rejected(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const orgID int64 = 88
	const userID int64 = 42

	pid := int64(201)
	price, _ := money.Parse("15.50")

	mockRepo := &mockCatalogMatchingRepo{
		mockCatalogImageRepo: newMockCatalogImageRepo(),
	}
	catSvc := catalog.NewService(mockRepo, logger)

	matchingMock := &mockRevalidateMatchingSvc{
		revalidateFn: func(_ context.Context, _ matching.Scope, _ matching.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
			// Catalog changed flag
			d.Flags = append(d.Flags, "catalog_changed")
			return d, nil
		},
	}
	catSvc.SetMatchingService(matchingMock)

	items := []*StagedSavingItem{
		{
			Index:             0,
			NameProduct:       "بانادول إكسترا",
			SKU:               "PAN-24",
			Quantity:          10,
			Price:             price,
			ProductID:         &pid,
			MasterProductName: "بانادول إكسترا 24 قرص",
			Included:          true,
		},
	}

	valid, rejected, err := RevalidateSavingItems(context.Background(), orgID, userID, items, catSvc)
	require.NoError(t, err)
	assert.Equal(t, 1, rejected)
	assert.Len(t, valid, 0)
}

func TestRevalidateSavingItems_UnconfirmedState_Rejected(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const orgID int64 = 88
	const userID int64 = 42

	pid := int64(201)
	price, _ := money.Parse("15.50")

	mockRepo := &mockCatalogMatchingRepo{
		mockCatalogImageRepo: newMockCatalogImageRepo(),
	}
	catSvc := catalog.NewService(mockRepo, logger)

	matchingMock := &mockRevalidateMatchingSvc{
		revalidateFn: func(_ context.Context, _ matching.Scope, _ matching.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
			// State is needs_review, no longer accepted/confirmed
			d.State = matchflow.StateNeedsReview
			return d, nil
		},
	}
	catSvc.SetMatchingService(matchingMock)

	items := []*StagedSavingItem{
		{
			Index:             0,
			NameProduct:       "بانادول إكسترا",
			SKU:               "PAN-24",
			Quantity:          10,
			Price:             price,
			ProductID:         &pid,
			MasterProductName: "بانادول إكسترا 24 قرص",
			Included:          true,
		},
	}

	valid, rejected, err := RevalidateSavingItems(context.Background(), orgID, userID, items, catSvc)
	require.NoError(t, err)
	assert.Equal(t, 1, rejected)
	assert.Len(t, valid, 0)
}
