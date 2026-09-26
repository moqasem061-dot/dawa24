package compare_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// scriptedAI proposes one product for every item and answers every pair check
// the same way.
type scriptedAI struct {
	mu        sync.Mutex
	propose   int64
	check     matchflow.Check
	proposals int
	checks    int
}

func (s *scriptedAI) Enhance(_ context.Context, b matchflow.Batch) ([]matchflow.Decision, error) {
	s.mu.Lock()
	s.proposals++
	s.mu.Unlock()
	out := make([]matchflow.Decision, 0, len(b.Items))
	for _, it := range b.Items {
		id := s.propose
		out = append(out, matchflow.Decision{Ref: it.Ref, ProductID: &id, Confidence: 0.95})
	}
	return out, nil
}

func (s *scriptedAI) Verify(_ context.Context, b matchflow.CheckBatch) ([]matchflow.Check, error) {
	s.mu.Lock()
	s.checks++
	s.mu.Unlock()
	out := make([]matchflow.Check, 0, len(b.Pairs))
	for _, p := range b.Pairs {
		c := s.check
		c.Ref = p.Ref
		out = append(out, c)
	}
	return out, nil
}

func newAIService(ai matchflow.Enhancer) *compare.Service {
	svc := compare.NewService(newMockCompareRepo(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetMatchEnhancer(ai)
	return svc
}

// The catalogue carries Panadol Extra and not Panadol. A model that names
// Panadol Extra for plain Panadol — and a pair check that, wrongly, agrees —
// still cannot put it on the row: the identity guard sees the line extension.
// This is the forced match the old per-row AI step made at sixty per cent.
func TestAIStageCannotForceALineExtension(t *testing.T) {
	ai := &scriptedAI{propose: 45, check: matchflow.Check{Brand: "same", Strength: "unstated",
		Form: "same", Variant: "same", Verdict: "same", Confidence: 0.99}}
	svc := newAIService(ai)

	rows := []*compare.CompareFileRow{{RawName: "بانادول 24 قرص"}}
	if err := svc.MatchFileRowsToCatalog(context.Background(), 1, rows); err != nil {
		t.Fatal(err)
	}
	if rows[0].MatchedProductID != nil && *rows[0].MatchedProductID == 45 {
		t.Fatalf("plain Panadol was matched to Panadol Extra (method %s)", rows[0].MatchMethod)
	}
}

// A proposal the pair check calls a different product is never applied.
func TestAIStageRefusesWhatThePairCheckRejects(t *testing.T) {
	ai := &scriptedAI{propose: 88, check: matchflow.Check{Brand: "different", Strength: "unstated",
		Form: "same", Variant: "same", Verdict: "different", Confidence: 0.97}}
	svc := newAIService(ai)

	rows := []*compare.CompareFileRow{{RawName: "كونجستيل اقراص"}}
	if err := svc.MatchFileRowsToCatalog(context.Background(), 1, rows); err != nil {
		t.Fatal(err)
	}
	if rows[0].MatchMethod == compare.MatchMethodAI {
		t.Fatalf("applied an AI match the pair check rejected: %v", rows[0].MatchedProductID)
	}
}

// A proposal both questions agree on is applied as an AI match.
func TestAIStageAppliesAVerifiedProposal(t *testing.T) {
	ai := &scriptedAI{propose: 88, check: matchflow.Check{Brand: "same", Strength: "unstated",
		Form: "same", Variant: "same", Verdict: "same", Confidence: 0.93}}
	svc := newAIService(ai)

	rows := []*compare.CompareFileRow{{RawName: "كونجستيل اقراص"}}
	if err := svc.MatchFileRowsToCatalog(context.Background(), 1, rows); err != nil {
		t.Fatal(err)
	}
	if rows[0].MatchedProductID == nil || *rows[0].MatchedProductID != 88 {
		t.Fatalf("verified proposal not applied: %v (method %s, proposals %d, checks %d)",
			rows[0].MatchedProductID, rows[0].MatchMethod, ai.proposals, ai.checks)
	}
}
