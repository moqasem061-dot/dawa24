package pipeline

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockSmartOrderMatcher struct {
	prepareFunc func(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error)
}

func (m *mockSmartOrderMatcher) Prepare(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
	if m.prepareFunc != nil {
		return m.prepareFunc(ctx, req)
	}
	return productmatch.PrepareResult{}, nil
}

func (m *mockSmartOrderMatcher) Rematch(ctx context.Context, s productmatch.Scope, row productmatch.Row, rev int) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}

func (m *mockSmartOrderMatcher) Revalidate(ctx context.Context, s productmatch.Scope, ref productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
	return d, nil
}

func TestCandidateMatcher_UsesPrepareWithPackGranularity(t *testing.T) {
	var capturedReq productmatch.PrepareRequest
	targetPID := int64(999)

	mock := &mockSmartOrderMatcher{
		prepareFunc: func(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
			capturedReq = req
			decisions := make([]matchflow.RowDecision, len(req.Rows))
			for i := range req.Rows {
				decisions[i] = matchflow.RowDecision{
					V:         1,
					State:     matchflow.StateAccepted,
					Basis:     matchflow.BasisSimilarity,
					ProductID: &targetPID,
					Granularity: matchflow.GranularityPair{
						Required: matchflow.GranularityPack,
						Achieved: matchflow.GranularityPack,
					},
					Revision: 1,
				}
			}
			return productmatch.PrepareResult{
				RunID:     42,
				Decisions: decisions,
			}, nil
		},
	}

	cm := NewCandidateMatcher(nil, 0.85)
	cm.SetService(mock)

	line := &smartorder.Line{
		ID:        10,
		RawName:   "بانادول 500 مجم 24 قرص",
		RowNumber: 1,
	}
	Normalize([]*smartorder.Line{line})

	reviews := cm.ScoreWithContext(context.Background(), 1001, 555, []*smartorder.Line{line})
	if len(reviews) != 0 {
		t.Fatalf("expected 0 reviews for accepted decision, got %d", len(reviews))
	}

	if capturedReq.Scope.Workflow != productmatch.WorkflowSmartOrder {
		t.Errorf("expected WorkflowSmartOrder, got %s", capturedReq.Scope.Workflow)
	}
	if capturedReq.Required != productmatch.GranularityPack {
		t.Errorf("expected GranularityPack, got %s", capturedReq.Required)
	}
	if capturedReq.Scope.EffectiveOrg != 1001 {
		t.Errorf("expected EffectiveOrg 1001, got %d", capturedReq.Scope.EffectiveOrg)
	}

	if !line.Matched() || line.MatchedProductID == nil || *line.MatchedProductID != targetPID {
		t.Errorf("expected line to match product %d, got %v", targetPID, line.MatchedProductID)
	}
	if line.MatchState != "accepted" {
		t.Errorf("expected MatchState accepted, got %s", line.MatchState)
	}
	if line.MatchGranularity != "pack" {
		t.Errorf("expected MatchGranularity pack, got %s", line.MatchGranularity)
	}
	if line.MatchRunID == nil || *line.MatchRunID != 42 {
		t.Errorf("expected MatchRunID 42, got %v", line.MatchRunID)
	}
	if len(line.MatchDecision) == 0 {
		t.Errorf("expected non-empty MatchDecision JSON")
	}
}

func TestCandidateMatcher_UnacceptedReturnsReview(t *testing.T) {
	mock := &mockSmartOrderMatcher{
		prepareFunc: func(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
			return productmatch.PrepareResult{
				Decisions: []matchflow.RowDecision{
					{
						V:     1,
						State: matchflow.StateNeedsReview,
						Basis: matchflow.BasisSimilarity,
						Candidates: []matchflow.CandidateDecision{
							{ProductID: 123, Rank: 1, Eligible: true},
						},
						Revision: 1,
					},
				},
			}, nil
		},
	}

	cm := NewCandidateMatcher(nil, 0.85)
	cm.SetService(mock)

	line := &smartorder.Line{
		ID:        20,
		RawName:   "كونجستال اقراص",
		RowNumber: 1,
	}
	Normalize([]*smartorder.Line{line})

	reviews := cm.ScoreWithContext(context.Background(), 1001, 555, []*smartorder.Line{line})
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if line.Matched() {
		t.Errorf("unaccepted line should not be matched")
	}
	if line.MatchState != "needs_review" {
		t.Errorf("expected MatchState needs_review, got %s", line.MatchState)
	}
}
