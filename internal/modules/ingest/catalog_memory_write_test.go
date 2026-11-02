package ingest

import (
	"context"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// An accepted model answer is written to the decision memory, not just applied.
//
// The stage used to apply it and file nothing: SaveDecisions on the other side was
// a no-op, so the next upload asked the same question again, and the supplier's
// ذاكرة قرارات المطابقة never showed the answer. This is the pipeline half of that
// contract — the stage hands the answer, with its question signature, to the
// memory — and cmd/server/pipeline_memory_test.go is the routing half, while
// test/integration/decision_memory_recall_test.go proves the next import obeys it.
func TestAcceptedAIAnswersAreHandedToTheMemory(t *testing.T) {
	q := question("بانادول", []int{2}, 22)
	mem := &memory{}
	ai := &fakeEnhancer{reply: func(EnhanceBatch) []EnhanceOutcome {
		return []EnhanceOutcome{answer(1, 22, 0.96)}
	}}

	e := NewEnhancement(ai, mem, testIndex(), nil)
	e.Run(context.Background(), []*openRow{q})

	if len(mem.saved) != 1 {
		t.Fatalf("saved %d decisions, want the 1 answer that was applied", len(mem.saved))
	}
	saved := mem.saved[0]
	if saved.ChosenProductID == nil || *saved.ChosenProductID != 22 {
		t.Errorf("saved product = %v, want 22", saved.ChosenProductID)
	}
	if saved.Key != decisionKey(q) {
		t.Errorf("saved under key %q, want the question's key %q", saved.Key, decisionKey(q))
	}
	if strings.TrimSpace(saved.NormName) == "" {
		t.Error("saved without a name; the memory row could never be searched for again")
	}
	if strings.TrimSpace(saved.Signature) == "" {
		t.Error("saved without a question signature; the matcher looks memory up by signature, so this answer is unreachable")
	}
}

// A refused answer is not remembered.
//
// The point of the identity guard is that a wrong answer never becomes the
// platform's answer: an answer naming a product that was not even in the row's
// shortlist is refused, and filing it would hand it to the next import even
// though this one refused it.
func TestRefusedAIAnswersAreNotRemembered(t *testing.T) {
	q := question("بانادول", []int{2}, 22)
	mem := &memory{}
	ai := &fakeEnhancer{reply: func(EnhanceBatch) []EnhanceOutcome {
		return []EnhanceOutcome{answer(1, 999, 0.99)} // never offered for this row
	}}

	e := NewEnhancement(ai, mem, testIndex(), nil)
	e.Run(context.Background(), []*openRow{q})

	if len(mem.saved) != 0 {
		t.Errorf("saved %d decisions for answers this run refused", len(mem.saved))
	}
}

// The commit step remembers the mappings it just confirmed.
//
// These are the rows a person accepted (or the engine settled outright) and the
// vendor then committed. They are written with source 'manual', which is what the
// import's own read path trusts on the next upload — so a hand-corrected file
// teaches the platform instead of being re-guessed every week.
func TestCommittedMatchesAreSavedToTheDecisionMemory(t *testing.T) {
	ctx := context.Background()
	productID := int64(101)
	row := stagedRow(1, &productID, "SKU-1", "بانادول 500 مجم", 5, true)
	row.MatchLevel = "exact"
	row.MatchState = string(matchflow.StateAccepted)
	row.MatchRevision = 1
	// What the review screen and the commit both read: the row's own name.
	row.DisplayName = "بانادول 500 مجم"
	// A person matched this row by hand. Only those are remembered: an
	// engine-accepted row is recomputable, and filed as a person's decision it
	// would outrank every future engine's evidence for that question for good.
	row.IsManuallyMatched = true

	keys := []VariantKey{{ID: 501, ProductID: productID, SKU: "SKU-1", Active: true}}
	st := DefaultSettings()
	st.WarehouseID = 1
	sess := session("imp-memory", st)

	svc, _, _, _ := commitFixture(sess, []*RowOutcome{row}, keys, map[int64]bool{501: true})
	mem := &memory{}
	svc.SetMatchMemory(mem)

	res, err := svc.CommitImport(ctx, sess.PublicID)
	if err != nil {
		t.Fatalf("CommitImport: %v", err)
	}
	if res.UpdatedRows == 0 {
		t.Fatalf("the commit wrote no row, so the memory assertion would prove nothing: %+v", res)
	}

	if len(mem.saved) == 0 {
		t.Fatal("a committed import wrote nothing to the decision memory; the next upload pays for it again")
	}
	saved := mem.saved[0]
	if saved.Source != "manual" {
		t.Errorf("source = %q, want manual: the import's read path only trusts a person's mapping", saved.Source)
	}
	if saved.ChosenProductID == nil || *saved.ChosenProductID != productID {
		t.Errorf("saved product = %v, want %d", saved.ChosenProductID, productID)
	}
	if !strings.HasPrefix(saved.Key, "manual:") {
		t.Errorf("key = %q, want the manual: prefix the read path looks up", saved.Key)
	}
}
