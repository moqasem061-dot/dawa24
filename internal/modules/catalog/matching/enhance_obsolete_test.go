package matching_test

import (
	"context"
	"sync"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

type memoryRowState struct {
	ID          int64
	Revision    int
	HumanLocked bool
	RunID       *int64
	Decision    matchflow.RowDecision
}

type memoryApplier struct {
	mu   sync.Mutex
	rows map[int64]*memoryRowState
}

func newMemoryApplier() *memoryApplier {
	return &memoryApplier{
		rows: make(map[int64]*memoryRowState),
	}
}

func (m *memoryApplier) SetRow(r *memoryRowState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[r.ID] = r
}

func (m *memoryApplier) GetRow(id int64) (memoryRowState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return memoryRowState{}, false
	}
	return *r, true
}

// ApplyAI implements matching.Applier with compare-and-set semantics:
// UPDATE ... SET match_revision = match_revision + 1, decision = d, run_id = runID
// WHERE id = ref.ID AND match_revision = expectedRev AND human_locked = false
func (m *memoryApplier) ApplyAI(ctx context.Context, ref matching.RowRef, expectedRev int, runID int64, d matchflow.RowDecision) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.rows[ref.ID]
	if !ok {
		return false, nil
	}
	// Enforce CAS condition: expectedRev must match current revision, and row must not be human-locked
	if r.Revision != expectedRev || r.HumanLocked {
		return false, nil
	}

	r.Revision++
	r.RunID = &runID
	d.Revision = r.Revision
	d.AI.State = matchflow.AIStateCompleted
	r.Decision = d
	return true, nil
}

// TestApplyAI_HumanEditDuringRun_MarksObsoleteAndLeavesRowUnchanged tests SC-009 / R13:
// A human edit increments match_revision during an AI run, rendering the late AI answer
// obsolete, returning applied=false, and preserving the human edit untouched.
func TestApplyAI_HumanEditDuringRun_MarksObsoleteAndLeavesRowUnchanged(t *testing.T) {
	ctx := context.Background()
	applier := newMemoryApplier()

	pOld := int64(100)
	pHuman := int64(200)
	pAI := int64(300)

	initialDec := matchflow.RowDecision{
		State:     matchflow.StateNeedsReview,
		ProductID: &pOld,
		Revision:  1,
	}

	// 1. Row starts at revision 1, unsettled
	applier.SetRow(&memoryRowState{
		ID:          42,
		Revision:    1,
		HumanLocked: false,
		Decision:    initialDec,
	})

	// 2. Planning phase captures (row_id=42, revision=1, run_id=999)
	plannedRev := 1
	runID := int64(999)

	// 3. Human reviewer edits the row before AI completes, bumping revision to 2
	applier.mu.Lock()
	r := applier.rows[42]
	r.Revision = 2
	r.HumanLocked = true
	r.Decision = matchflow.RowDecision{
		State:     matchflow.StateConfirmed,
		ProductID: &pHuman,
		Revision:  2,
		Basis:     matchflow.BasisReviewer,
	}
	applier.mu.Unlock()

	// 4. AI run finishes with an answer for product 300, attempting to apply with expectedRev=1
	aiDec := matchflow.RowDecision{
		State:     matchflow.StateAccepted,
		ProductID: &pAI,
		Basis:     matchflow.BasisAI,
	}

	ref := matching.RowRef{ID: 42}
	applied, err := applier.ApplyAI(ctx, ref, plannedRev, runID, aiDec)
	if err != nil {
		t.Fatalf("unexpected error from ApplyAI: %v", err)
	}

	// 5. Assert: applied is false (obsolete)
	if applied {
		t.Fatal("expected ApplyAI to return false on obsolete revision, got true")
	}

	// 6. Assert: row remains unchanged with the human reviewer's decision
	finalRow, ok := applier.GetRow(42)
	if !ok {
		t.Fatal("expected row to exist")
	}
	if finalRow.Revision != 2 {
		t.Errorf("expected revision 2, got %d", finalRow.Revision)
	}
	if finalRow.Decision.ProductID == nil || *finalRow.Decision.ProductID != pHuman {
		t.Errorf("expected human product %d preserved, got %v", pHuman, finalRow.Decision.ProductID)
	}
	if finalRow.Decision.State != matchflow.StateConfirmed {
		t.Errorf("expected state confirmed, got %s", finalRow.Decision.State)
	}
}

// TestApplyAI_HumanLocked_ReturnsFalse verifies that a human-locked row cannot be overwritten by AI.
func TestApplyAI_HumanLocked_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	applier := newMemoryApplier()

	pUser := int64(500)
	applier.SetRow(&memoryRowState{
		ID:          10,
		Revision:    3,
		HumanLocked: true, // locked
		Decision: matchflow.RowDecision{
			State:     matchflow.StateConfirmed,
			ProductID: &pUser,
			Revision:  3,
		},
	})

	pAI := int64(999)
	aiDec := matchflow.RowDecision{
		State:     matchflow.StateAccepted,
		ProductID: &pAI,
	}

	applied, err := applier.ApplyAI(ctx, matching.RowRef{ID: 10}, 3, 100, aiDec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied {
		t.Fatal("expected ApplyAI to return false for human-locked row")
	}

	row, _ := applier.GetRow(10)
	if row.Decision.ProductID == nil || *row.Decision.ProductID != pUser {
		t.Errorf("expected user product %d preserved, got %v", pUser, row.Decision.ProductID)
	}
}

// TestApplyAI_MatchingRevision_AppliesSuccessfully verifies successful CAS when revision matches.
func TestApplyAI_MatchingRevision_AppliesSuccessfully(t *testing.T) {
	ctx := context.Background()
	applier := newMemoryApplier()

	applier.SetRow(&memoryRowState{
		ID:          20,
		Revision:    1,
		HumanLocked: false,
		Decision: matchflow.RowDecision{
			State:    matchflow.StateNeedsReview,
			Revision: 1,
		},
	})

	pAI := int64(777)
	aiDec := matchflow.RowDecision{
		State:     matchflow.StateAccepted,
		ProductID: &pAI,
		Basis:     matchflow.BasisAI,
	}

	applied, err := applier.ApplyAI(ctx, matching.RowRef{ID: 20}, 1, 200, aiDec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !applied {
		t.Fatal("expected ApplyAI to succeed with matching revision")
	}

	row, _ := applier.GetRow(20)
	if row.Revision != 2 {
		t.Errorf("expected revision incremented to 2, got %d", row.Revision)
	}
	if row.Decision.ProductID == nil || *row.Decision.ProductID != pAI {
		t.Errorf("expected AI product %d applied, got %v", pAI, row.Decision.ProductID)
	}
	if row.Decision.AI.State != matchflow.AIStateCompleted {
		t.Errorf("expected AI state completed, got %s", row.Decision.AI.State)
	}
}
