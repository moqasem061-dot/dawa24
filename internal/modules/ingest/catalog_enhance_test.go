package ingest

// What the AI stage is allowed to do, and what it must refuse.
//
// These tests are about the guards rather than about matching quality. Quality
// is the model's and is measured against live files; the guards are ours, and
// each one here exists because the failure it prevents ties a vendor's price to
// the wrong medicine — which nobody notices until a pharmacy receives it.

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// testIndex is a catalogue of three products that differ in the three ways the
// identity guard cares about: strength, line extension, and nothing at all.
func testIndex() *productmatch.Index {
	return productmatch.NewIndex([]productmatch.MasterProduct{
		{ID: 11, NameAR: "بانادول اكسترا", NameEN: "panadol extra", Concentration: "500 مجم"},
		{ID: 22, NameAR: "بانادول", NameEN: "panadol", Concentration: "500 مجم"},
		{ID: 33, NameAR: "اكسامايد", NameEN: "examide", Concentration: "10 مجم"},
	})
}

// fakeEnhancer answers with whatever it was told to, and records what it saw.
type fakeEnhancer struct {
	reply  func(EnhanceBatch) []EnhanceOutcome
	calls  int
	items  int
	window int
}

func (f *fakeEnhancer) Enhance(_ context.Context, b EnhanceBatch) ([]EnhanceOutcome, error) {
	f.calls++
	f.items += len(b.Items)
	f.window += len(b.Catalog)
	return f.reply(b), nil
}

// question builds one open row with a shortlist, bypassing retrieval.
func question(name string, rows []int, candidates ...int64) *openRow {
	q := &openRow{
		row:        &productmatch.Row{Number: rows[0], Name: name},
		sourceRows: rows,
		normName:   productmatch.NormalizeText(name),
	}
	for _, id := range candidates {
		q.candidates = append(q.candidates, productmatch.MatchCandidate{ProductID: id, Name: name})
	}
	return q
}

func answer(ref int, productID int64, confidence float64) EnhanceOutcome {
	out := EnhanceOutcome{Ref: ref, Confidence: confidence}
	if productID > 0 {
		out.ProductID = &productID
	}
	return out
}

func run(t *testing.T, ai Enhancer, rows ...*openRow) (*Enhancement, []AIMatch) {
	t.Helper()
	e := NewEnhancement(ai, nil, testIndex(), nil)
	return e, e.Run(context.Background(), rows)
}

// A product the model was never shown must never become a match. This is the
// guard between a hallucinated id and a live price.
func TestAnswerOutsideTheWindowIsRefused(t *testing.T) {
	ai := &fakeEnhancer{reply: func(EnhanceBatch) []EnhanceOutcome {
		return []EnhanceOutcome{answer(1, 999, 0.99)}
	}}
	e, matches := run(t, ai, question("بانادول", []int{2}, 22))

	if len(matches) != 0 {
		t.Fatalf("an id outside the window was applied: %+v", matches)
	}
	if e.Stats.Rejected != 1 {
		t.Errorf("rejected = %d, want 1", e.Stats.Rejected)
	}
}

// The prompt asks the model to abstain below the floor. An instruction is not a
// guarantee, so the floor is enforced here too.
func TestAnswerBelowTheConfidenceFloorIsNotApplied(t *testing.T) {
	ai := &fakeEnhancer{reply: func(EnhanceBatch) []EnhanceOutcome {
		return []EnhanceOutcome{answer(1, 22, ceilings.MinApplyConfidence-0.01)}
	}}
	e, matches := run(t, ai, question("بانادول", []int{2}, 22))

	if len(matches) != 0 {
		t.Fatalf("a low-confidence answer was applied: %+v", matches)
	}
	if e.Stats.Abstained != 1 {
		t.Errorf("abstained = %d, want 1", e.Stats.Abstained)
	}
}

// "بانادول" and "بانادول اكسترا" are different products that share a word. The
// model is instructed at length about this and is usually right; the guard is
// what makes "usually" acceptable.
func TestIdentityConflictOverrulesTheModel(t *testing.T) {
	ai := &fakeEnhancer{reply: func(EnhanceBatch) []EnhanceOutcome {
		return []EnhanceOutcome{answer(1, 11, 0.97)} // بانادول اكسترا
	}}
	e, matches := run(t, ai, question("بانادول", []int{2}, 11, 22))

	if len(matches) != 0 {
		t.Fatalf("a line-extension mismatch was applied: %+v", matches)
	}
	if e.Stats.Rejected != 1 || len(e.Stats.RefusedBy) == 0 {
		t.Errorf("refusal not recorded: rejected=%d by=%v", e.Stats.Rejected, e.Stats.RefusedBy)
	}
}

// A price list that names one product in four warehouses asks one question and
// pays once — and all four rows receive the answer.
func TestRepeatedRowsAskOnceAndAllReceiveTheAnswer(t *testing.T) {
	ai := &fakeEnhancer{reply: func(EnhanceBatch) []EnhanceOutcome {
		return []EnhanceOutcome{answer(1, 22, 0.95)}
	}}
	e, matches := run(t, ai, question("بانادول", []int{2, 7, 9, 14}, 22))

	if ai.items != 1 {
		t.Errorf("items sent = %d, want 1", ai.items)
	}
	if len(matches) != 4 {
		t.Fatalf("matches = %d, want 4 (one per row that asked)", len(matches))
	}
	if e.Stats.Improved != 4 {
		t.Errorf("improved = %d, want 4", e.Stats.Improved)
	}
}

// Candidates shared between questions are sent once. This is the saving that
// makes a long supplier file cost the same as a short one.
func TestOverlappingShortlistsShareOneCatalogueWindow(t *testing.T) {
	ai := &fakeEnhancer{reply: func(EnhanceBatch) []EnhanceOutcome { return nil }}
	run(t, ai,
		question("بانادول", []int{2}, 11, 22),
		question("بانادول اكسترا", []int{3}, 11, 22),
	)

	if ai.calls != 1 {
		t.Fatalf("requests = %d, want 1", ai.calls)
	}
	if ai.window != 2 {
		t.Errorf("catalogue rows sent = %d, want 2 — the window is not de-duplicated", ai.window)
	}
}

// memory is a decision cache that answers from a map.
type memory struct {
	answers map[string]CachedDecision
	saved   []CachedDecision
}

func (m *memory) LookupDecisions(_ context.Context, keys []string) (map[string]CachedDecision, error) {
	out := map[string]CachedDecision{}
	for _, k := range keys {
		if d, ok := m.answers[k]; ok {
			out[k] = d
		}
	}
	return out, nil
}

func (m *memory) SaveDecisions(_ context.Context, d []CachedDecision) error {
	m.saved = append(m.saved, d...)
	return nil
}

func (m *memory) SaveAlias(context.Context, int64, string, string, float64) error { return nil }

// A remembered answer settles the row without a request. This is what makes a
// vendor's weekly re-upload nearly free.
func TestRememberedAnswersCostNothing(t *testing.T) {
	q := question("بانادول", []int{2}, 22)
	id := int64(22)
	mem := &memory{answers: map[string]CachedDecision{
		decisionKey(q): {ChosenProductID: &id, Confidence: 0.95, PromptVersion: PromptVersion},
	}}

	ai := &fakeEnhancer{reply: func(EnhanceBatch) []EnhanceOutcome {
		t.Error("a remembered question was sent to the model")
		return nil
	}}
	e := NewEnhancement(ai, mem, testIndex(), nil)
	matches := e.Run(context.Background(), []*openRow{q})

	if ai.calls != 0 {
		t.Errorf("requests = %d, want 0", ai.calls)
	}
	if len(matches) != 1 || matches[0].ProductID != 22 {
		t.Fatalf("cached answer not applied: %+v", matches)
	}
	if e.Stats.CacheHits != 1 {
		t.Errorf("cache hits = %d, want 1", e.Stats.CacheHits)
	}
}

// The cache key is the question, not the row: the same text against a different
// shortlist is a different question and must not reuse the answer.
func TestDecisionKeyDependsOnTheShortlist(t *testing.T) {
	a := decisionKey(question("بانادول", []int{2}, 22))
	b := decisionKey(question("بانادول", []int{2}, 22, 11))
	if a == b {
		t.Error("two different shortlists produced one cache key")
	}
	// Order of retrieval must not matter; the same options are the same question.
	if decisionKey(question("بانادول", []int{2}, 11, 22)) != b {
		t.Error("candidate order changed the cache key")
	}
}

// A run stops at its request ceiling and says so, rather than spending without
// limit or dropping rows silently.
func TestRequestCeilingStopsTheRunAndIsReported(t *testing.T) {
	rows := make([]*openRow, 0, (ceilings.MaxRequestsPerRun+2)*ceilings.MaxItemsPerRequest)
	for i := 0; i < cap(rows); i++ {
		rows = append(rows, question(productName(i), []int{i + 2}, 22))
	}
	ai := &fakeEnhancer{reply: func(EnhanceBatch) []EnhanceOutcome { return nil }}
	e := NewEnhancement(ai, nil, testIndex(), nil)
	e.Run(context.Background(), rows)

	if ai.calls > ceilings.MaxRequestsPerRun {
		t.Errorf("requests = %d, over the ceiling of %d", ai.calls, ceilings.MaxRequestsPerRun)
	}
	if !e.Stats.CeilingHit {
		t.Error("the ceiling was reached and not reported")
	}
}

// productName makes distinct names so nothing collapses by accident.
func productName(i int) string {
	const letters = "أبتثجحخدذرزسشصضطظعغفقكلمنهوي"
	r := []rune(letters)
	return string(r[i%len(r)]) + string(r[(i/len(r))%len(r)]) + string(r[(i/(len(r)*len(r)))%len(r)])
}
