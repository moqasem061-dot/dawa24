package matching

import (
	"context"
	"math/rand"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockAuditApplier struct {
	appliedDec matchflow.RowDecision
	called     bool
}

func (m *mockAuditApplier) ApplyAI(ctx context.Context, ref RowRef, expectedRev int, runID int64, d matchflow.RowDecision) (bool, error) {
	m.called = true
	m.appliedDec = d
	return true, nil
}

func TestSampleAuditRows_ZeroAndNegative(t *testing.T) {
	rows := []EnhanceRow{
		{
			RowRef: RowRef{ID: 1},
			Decision: matchflow.RowDecision{
				State:     matchflow.StateAccepted,
				ProductID: ptrInt64(101),
			},
		},
	}
	if got := SampleAuditRows(rows, 0, nil); len(got) != 0 {
		t.Fatalf("expected 0 sampled, got %d", len(got))
	}
	if got := SampleAuditRows(rows, -0.5, nil); len(got) != 0 {
		t.Fatalf("expected 0 sampled, got %d", len(got))
	}
}

func TestSampleAuditRows_FiltersUnsettled(t *testing.T) {
	rows := []EnhanceRow{
		{
			RowRef: RowRef{ID: 1},
			Decision: matchflow.RowDecision{
				State:     matchflow.StateNeedsReview,
				ProductID: ptrInt64(101),
			},
		},
		{
			RowRef: RowRef{ID: 2},
			Decision: matchflow.RowDecision{
				State:     matchflow.StateAccepted,
				ProductID: ptrInt64(102),
			},
		},
		{
			RowRef: RowRef{ID: 3},
			Decision: matchflow.RowDecision{
				State:     matchflow.StateConfirmed,
				ProductID: ptrInt64(103),
			},
		},
	}

	rng := rand.New(rand.NewSource(1))
	sampled := SampleAuditRows(rows, 1.0, rng)
	if len(sampled) != 2 {
		t.Fatalf("expected 2 sampled rows, got %d", len(sampled))
	}
	if sampled[0].RowRef.ID != 2 || sampled[1].RowRef.ID != 3 {
		t.Fatalf("unexpected sampled IDs: %v", sampled)
	}
}

func TestAuditRows_AIDisagrees_FlagsWithoutChangingTarget(t *testing.T) {
	ctx := context.Background()
	targetID := int64(101)
	row := EnhanceRow{
		RowRef: RowRef{ID: 1, RowNumber: 1},
		Row:    productmatch.Row{Name: "Concor 5mg"},
		Decision: matchflow.RowDecision{
			State:     matchflow.StateAccepted,
			Basis:     matchflow.BasisExact,
			ProductID: &targetID,
			Revision:  1,
		},
	}

	// Fake enhancer returning disagreement (ProductID: nil -> none of these)
	disagreeEnhancer := matchflow.EnhancerFunc(func(ctx context.Context, batch matchflow.Batch) ([]matchflow.Decision, error) {
		return []matchflow.Decision{
			{
				Ref:       0,
				ProductID: nil,
				Reason:    "differs in dosage",
			},
		}, nil
	})

	applier := &mockAuditApplier{}
	auditor := NewAuditAuditor(disagreeEnhancer)

	disagreements, err := auditor.AuditRows(ctx, 42, []EnhanceRow{row}, applier)
	if err != nil {
		t.Fatalf("unexpected audit error: %v", err)
	}
	if disagreements != 1 {
		t.Fatalf("expected 1 disagreement, got %d", disagreements)
	}
	if !applier.called {
		t.Fatalf("expected applier to be called")
	}

	// Verify target product ID unchanged
	if applier.appliedDec.ProductID == nil || *applier.appliedDec.ProductID != targetID {
		t.Fatalf("expected target product ID %d, got %v", targetID, applier.appliedDec.ProductID)
	}

	// Verify ai_disagrees flag is present
	foundFlag := false
	for _, f := range applier.appliedDec.Flags {
		if f == FlagAIDisagrees {
			foundFlag = true
			break
		}
	}
	if !foundFlag {
		t.Fatalf("expected %s flag in %v", FlagAIDisagrees, applier.appliedDec.Flags)
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}
