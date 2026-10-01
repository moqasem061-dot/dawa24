package catalog

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockScoreMatchingService struct {
	prepareFn func(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error)
}

func (m *mockScoreMatchingService) Prepare(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
	if m.prepareFn != nil {
		return m.prepareFn(ctx, req)
	}
	return productmatch.PrepareResult{}, nil
}

func (m *mockScoreMatchingService) GetRun(context.Context, int64) (*matching.MatchRun, error) {
	return nil, nil
}
func (m *mockScoreMatchingService) ListRunRows(context.Context, matching.Scope, int64, int) ([]matching.DiagnosticRowData, error) {
	return nil, nil
}
func (m *mockScoreMatchingService) MarkReviewable(context.Context, int64) error { return nil }
func (m *mockScoreMatchingService) Rematch(context.Context, matching.Scope, productmatch.Row, int) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}
func (m *mockScoreMatchingService) Search(context.Context, matching.Scope, string, productmatch.Row, int) ([]matchflow.CandidateView, error) {
	return nil, nil
}
func (m *mockScoreMatchingService) Review(context.Context, matching.Scope, matching.ReviewAction) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}
func (m *mockScoreMatchingService) PreviewBulk(context.Context, matching.Scope, matching.ReviewAction, []matching.RowRef) (matching.BulkPreview, error) {
	return matching.BulkPreview{}, nil
}
func (m *mockScoreMatchingService) StartEnhancement(context.Context, int64, []matching.EnhanceRow) (matching.EnhanceHandle, error) {
	return matching.EnhanceHandle{}, nil
}
func (m *mockScoreMatchingService) StopEnhancement(context.Context, int64) error { return nil }
func (m *mockScoreMatchingService) Revalidate(context.Context, matching.Scope, matching.RowRef, matchflow.RowDecision) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}
func (m *mockScoreMatchingService) Remember(context.Context, matching.Scope, matching.KnowledgeWrite) error {
	return nil
}
func (m *mockScoreMatchingService) ListKnowledge(context.Context, matching.Scope, matching.KnowledgeFilter) (matching.Page[matching.Knowledge], error) {
	return matching.Page[matching.Knowledge]{}, nil
}
func (m *mockScoreMatchingService) RevokeKnowledge(context.Context, matching.Scope, int64) error {
	return nil
}
func (m *mockScoreMatchingService) PromoteKnowledge(context.Context, matching.Scope, int64) error {
	return nil
}
func (m *mockScoreMatchingService) Diagnose(context.Context, matching.Scope, int64, productmatch.Row, matchflow.RowDecision) (matching.DiagnosticRow, error) {
	return matching.DiagnosticRow{}, nil
}
func (m *mockScoreMatchingService) RegisterApplier(matching.Workflow, matching.Applier) {}
func (m *mockScoreMatchingService) GetApplier(matching.Workflow) matching.Applier       { return nil }
func (m *mockScoreMatchingService) ListAIConfigurations(context.Context) ([]matching.AIConfiguration, error) { return nil, nil }
func (m *mockScoreMatchingService) GetAIConfiguration(context.Context, string) (*matching.AIConfiguration, error) { return nil, nil }
func (m *mockScoreMatchingService) UpdateAIConfigurationStatus(context.Context, string, string, string, string) error { return nil }
func (m *mockScoreMatchingService) ListPolicies(context.Context) ([]matching.PolicyRecord, error) { return nil, nil }
func (m *mockScoreMatchingService) ListAssignments(context.Context) ([]matching.PolicyAssignment, error) { return nil, nil }
func (m *mockScoreMatchingService) UpdateAssignment(context.Context, string, string, string, *string, string, string) error { return nil }
func (m *mockScoreMatchingService) RollbackAssignment(context.Context, string, string, string) error { return nil }


func TestScoreRows_CallsMatchingServicePrepare(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var capturedReq productmatch.PrepareRequest
	prodID := int64(999)
	mock := &mockScoreMatchingService{
		prepareFn: func(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
			capturedReq = req
			decisions := make([]matchflow.RowDecision, len(req.Rows))
			for i := range req.Rows {
				decisions[i] = matchflow.RowDecision{
					V:         1,
					State:     matchflow.StateAccepted,
					Basis:     matchflow.BasisSimilarity,
					ProductID: &prodID,
					Granularity: matchflow.GranularityPair{
						Required: matchflow.GranularityProduct,
						Achieved: matchflow.GranularityProduct,
					},
				}
			}
			return productmatch.PrepareResult{
				Decisions: decisions,
			}, nil
		},
	}

	rows := []*productmatch.Row{
		{Name: "Panadol 500mg"},
		{Name: "Brufen 400mg"},
	}

	doneCount := 0
	results, err := scoreRows(ctx, logger, mock, nil, rows, productmatch.DefaultMatchOptions(), func(scored int) {
		doneCount = scored
	})
	if err != nil {
		t.Fatalf("scoreRows failed: %v", err)
	}

	if capturedReq.Required != productmatch.GranularityProduct {
		t.Errorf("expected required granularity product, got %v", capturedReq.Required)
	}
	if capturedReq.Scope.Workflow != productmatch.WorkflowCatalog {
		t.Errorf("expected workflow catalog, got %v", capturedReq.Scope.Workflow)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ProductID != 999 {
		t.Errorf("expected product ID 999, got %d", results[0].ProductID)
	}
	if results[0].Level != productmatch.MatchStrong {
		t.Errorf("expected match strong, got %v", results[0].Level)
	}
	if doneCount != 2 {
		t.Errorf("expected doneCount 2, got %d", doneCount)
	}
}
