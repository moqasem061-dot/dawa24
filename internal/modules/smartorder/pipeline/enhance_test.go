package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// The AI stage is the only part of matching that can be wrong in a way the
// buyer cannot see. Every test here is about a guard that keeps it honest.

// enhanceRepo records what was cached and answers cache lookups.
type enhanceRepo struct {
	smartorder.Repository
	cached  map[string]smartorder.CachedDecision
	saved   []smartorder.CachedDecision
	aliases int
}

func newEnhanceRepo() *enhanceRepo {
	return &enhanceRepo{cached: map[string]smartorder.CachedDecision{}}
}

func (r *enhanceRepo) LookupDecisions(_ context.Context, keys []string) (map[string]smartorder.CachedDecision, error) {
	out := map[string]smartorder.CachedDecision{}
	for _, k := range keys {
		if d, ok := r.cached[k]; ok {
			out[k] = d
		}
	}
	return out, nil
}

func (r *enhanceRepo) SaveDecisions(_ context.Context, d []smartorder.CachedDecision) error {
	r.saved = append(r.saved, d...)
	return nil
}

func (r *enhanceRepo) SaveAlias(context.Context, int64, string, string, float64) error {
	r.aliases++
	return nil
}

// stubEnhancer answers with whatever the test tells it to, and records what it
// was asked.
type stubEnhancer struct {
	batches []EnhanceBatch
	answer  func(EnhanceBatch) ([]EnhanceOutcome, error)
}

func (s *stubEnhancer) Enhance(_ context.Context, b EnhanceBatch) ([]EnhanceOutcome, error) {
	s.batches = append(s.batches, b)
	if s.answer == nil {
		return nil, nil
	}
	return s.answer(b)
}

// testIndex builds a small catalogue whose products differ in the ways that
// matter: strength, form, and Arabic spelling of the same Latin brand.
func testIndex() *productmatch.Index {
	return productmatch.NewIndex([]productmatch.MasterProduct{
		{ID: 101, NameAR: "ابيليفاي 10مجم 10 اقراص", NameEN: "abilify 10 mg 10 tabs",
			DosageForm: "tablet", Concentration: "10 mg"},
		{ID: 102, NameAR: "ابيليفاي 30مجم 10 اقراص", NameEN: "abilify 30 mg 10 tabs",
			DosageForm: "tablet", Concentration: "30 mg"},
		{ID: 103, NameAR: "ارموويك 50مجم 10 اقراص", NameEN: "armowic 50 mg 10 tabs",
			DosageForm: "tablet", Concentration: "50 mg"},
	})
}

func reviewFor(id int64, raw string, candidates ...int64) Review {
	l := &smartorder.Line{ID: id, RowNumber: int(id), RawName: raw}
	Normalize([]*smartorder.Line{l})
	r := Review{Line: l, Row: BuildRow(l)}
	for _, c := range candidates {
		r.Candidates = append(r.Candidates, matchCandidate(c, "منتج"))
	}
	return r
}

func runEnhancement(t *testing.T, repo *enhanceRepo, ai *stubEnhancer, reviews []Review) *Enhancement {
	t.Helper()
	e := NewEnhancement(repo, ai, testIndex(), nil)
	e.Run(context.Background(), reviews)
	return e
}

// 🚦 The guard that matters most: a product the model was never shown must never
// reach a purchase order, however confidently it was named.
func TestEnhancementRejectsAProductItNeverOffered(t *testing.T) {
	repo := newEnhanceRepo()
	invented := int64(999999)
	ai := &stubEnhancer{answer: func(b EnhanceBatch) ([]EnhanceOutcome, error) {
		return []EnhanceOutcome{{Ref: b.Items[0].Ref, ProductID: &invented, Confidence: 0.99}}, nil
	}}

	r := reviewFor(1, "ابليفاى 10مجم", 101)
	e := runEnhancement(t, repo, ai, []Review{r})

	if r.Line.Matched() {
		t.Fatalf("line was matched to an invented product %d", *r.Line.MatchedProductID)
	}
	if e.Stats.Rejected != 1 {
		t.Errorf("rejected = %d, want 1", e.Stats.Rejected)
	}
	if len(repo.saved) != 0 {
		t.Errorf("an invented answer was written to the decision cache")
	}
}

// A candidate retrieved for a *different* line is still a legitimate answer:
// the window is shared on purpose, and the commonest retrieval failure is that
// the right product came back for the row above.
func TestEnhancementAcceptsAnyProductInTheSharedWindow(t *testing.T) {
	repo := newEnhanceRepo()
	other := int64(103)
	ai := &stubEnhancer{answer: func(b EnhanceBatch) ([]EnhanceOutcome, error) {
		var out []EnhanceOutcome
		for _, it := range b.Items {
			if it.Text == "ارمو ويك50مجم" {
				out = append(out, EnhanceOutcome{Ref: it.Ref, ProductID: &other, Confidence: 0.95})
				continue
			}
			out = append(out, EnhanceOutcome{Ref: it.Ref, Confidence: 0.2})
		}
		return out, nil
	}}

	// The armowic line retrieved nothing of its own; 103 is in the window only
	// because the other line retrieved it.
	target := reviewFor(1, "ارمو ويك50مجم")
	donor := reviewFor(2, "ارموويك 50مجم", 103)
	e := runEnhancement(t, repo, ai, []Review{target, donor})

	if !target.Line.Matched() || *target.Line.MatchedProductID != 103 {
		t.Fatalf("line was not matched from the shared window: %+v", target.Line.MatchedProductID)
	}
	if target.Line.MatchMethod != smartorder.MethodAI {
		t.Errorf("method = %q, want %q", target.Line.MatchMethod, smartorder.MethodAI)
	}
	if e.Stats.Improved != 1 {
		t.Errorf("improved = %d, want 1", e.Stats.Improved)
	}
}

// An answer below the floor is remembered but not applied. The prompt asks the
// model to abstain there; this enforces it, because an instruction is not a
// guarantee.
func TestEnhancementDoesNotApplyBelowTheConfidenceFloor(t *testing.T) {
	repo := newEnhanceRepo()
	id := int64(101)
	ai := &stubEnhancer{answer: func(b EnhanceBatch) ([]EnhanceOutcome, error) {
		return []EnhanceOutcome{{Ref: b.Items[0].Ref, ProductID: &id, Confidence: ceilings.MinApplyConfidence - 0.01}}, nil
	}}

	r := reviewFor(1, "ابليفاى 10مجم", 101)
	e := runEnhancement(t, repo, ai, []Review{r})

	if r.Line.Matched() {
		t.Fatalf("a below-floor answer was applied")
	}
	if e.Stats.Abstained != 1 {
		t.Errorf("abstained = %d, want 1", e.Stats.Abstained)
	}
	// It is remembered as what the model actually said — the id and the
	// confidence — rather than blanked into an abstention.
	//
	// The floor differs between tools: the master-catalogue import demands 0.90
	// where the smart order demands 0.80, because a wrong match there overwrites
	// the entry every pharmacy reads. A cache row that had already discarded the
	// id could not serve both, and the tool with the lower floor would re-ask a
	// question that had been paid for.
	if len(repo.saved) != 1 {
		t.Fatalf("the answer was not remembered: %+v", repo.saved)
	}
	if repo.saved[0].ChosenProductID == nil || *repo.saved[0].ChosenProductID != id {
		t.Errorf("the remembered answer lost the product the model named: %+v", repo.saved[0])
	}
	if repo.saved[0].Confidence >= ceilings.MinApplyConfidence {
		t.Errorf("the remembered answer lost the confidence that keeps it unapplied: %+v",
			repo.saved[0])
	}
}

// 🚦 Strength is re-checked deterministically. A model that answers 30 mg for a
// 10 mg line is confidently prescribing a different dose, and no confidence
// score makes that acceptable.
func TestEnhancementRejectsAStrengthConflict(t *testing.T) {
	repo := newEnhanceRepo()
	wrongDose := int64(102) // 30 mg
	ai := &stubEnhancer{answer: func(b EnhanceBatch) ([]EnhanceOutcome, error) {
		return []EnhanceOutcome{{Ref: b.Items[0].Ref, ProductID: &wrongDose, Confidence: 0.99}}, nil
	}}

	r := reviewFor(1, "ابليفاى 10مجم", 101, 102)
	e := runEnhancement(t, repo, ai, []Review{r})

	if r.Line.Matched() {
		t.Fatalf("a 10 mg line was matched to a 30 mg product")
	}
	if e.Stats.Rejected != 1 {
		t.Errorf("rejected = %d, want 1", e.Stats.Rejected)
	}
}

// Two lines carrying the same text with the same shortlist are one question.
// Asking twice is money spent to learn nothing.
func TestEnhancementAsksOnceForDuplicateLines(t *testing.T) {
	repo := newEnhanceRepo()
	id := int64(101)
	ai := &stubEnhancer{answer: func(b EnhanceBatch) ([]EnhanceOutcome, error) {
		var out []EnhanceOutcome
		for _, it := range b.Items {
			out = append(out, EnhanceOutcome{Ref: it.Ref, ProductID: &id, Confidence: 0.95})
		}
		return out, nil
	}}

	a := reviewFor(1, "ابليفاى 10مجم", 101)
	b := reviewFor(2, "ابليفاى 10مجم", 101)
	e := runEnhancement(t, repo, ai, []Review{a, b})

	if len(ai.batches) != 1 {
		t.Fatalf("requests = %d, want 1", len(ai.batches))
	}
	if got := len(ai.batches[0].Items); got != 1 {
		t.Errorf("items sent = %d, want 1 — the duplicate was not collapsed", got)
	}
	for _, r := range []Review{a, b} {
		if !r.Line.Matched() || *r.Line.MatchedProductID != id {
			t.Errorf("line %d did not receive the shared answer", r.Line.ID)
		}
	}
	if e.Stats.Improved != 2 {
		t.Errorf("improved = %d, want 2 — both lines were improved", e.Stats.Improved)
	}
}

// A whole file's residue goes in one request when it fits. The previous design
// sent twenty-five rows at a time and never finished; this is the property that
// changed.
func TestEnhancementSendsOneRequestWhenItFits(t *testing.T) {
	repo := newEnhanceRepo()
	ai := &stubEnhancer{}

	var reviews []Review
	for i := int64(1); i <= 120; i++ {
		reviews = append(reviews, reviewFor(i, "صنف رقم "+string(rune('أ'+i%20)), 101, 102, 103))
	}
	runEnhancement(t, repo, ai, reviews)

	if len(ai.batches) != 1 {
		t.Fatalf("requests = %d, want 1", len(ai.batches))
	}
	// The catalogue window carries each product once however many items cite it.
	if got := len(ai.batches[0].Catalog); got != 3 {
		t.Errorf("catalogue window = %d rows, want 3 — candidates were not de-duplicated", got)
	}
}

// A cached decision is applied without a request. This is what makes the second
// import of a weekly file nearly free.
func TestEnhancementAppliesCachedDecisionsWithoutAsking(t *testing.T) {
	repo := newEnhanceRepo()
	ai := &stubEnhancer{}

	r := reviewFor(1, "ابليفاى 10مجم", 101)
	id := int64(101)
	repo.cached[keyOf(r)] = smartorder.CachedDecision{
		ChosenProductID: &id, Confidence: 0.93, PromptVersion: PromptVersion,
	}

	e := runEnhancement(t, repo, ai, []Review{r})

	if len(ai.batches) != 0 {
		t.Fatalf("a cached line was still sent for a decision")
	}
	if !r.Line.Matched() || *r.Line.MatchedProductID != 101 {
		t.Fatalf("the cached decision was not applied")
	}
	if e.Stats.CacheHits != 1 {
		t.Errorf("cache hits = %d, want 1", e.Stats.CacheHits)
	}
}

// A failing Gateway must leave the deterministic result standing. A pharmacy
// has to be able to order when the AI is down.
func TestEnhancementFailureLeavesDeterministicResultsStanding(t *testing.T) {
	repo := newEnhanceRepo()
	ai := &stubEnhancer{answer: func(EnhanceBatch) ([]EnhanceOutcome, error) {
		return nil, errors.New("gateway unavailable")
	}}

	r := reviewFor(1, "ابليفاى 10مجم", 101)
	fuzzy := int64(102)
	r.Line.MatchedProductID = &fuzzy
	r.Line.MatchMethod = smartorder.MethodFuzzy
	r.Line.MatchConfidence = 0.42

	runEnhancement(t, repo, ai, []Review{r})

	if r.Line.MatchMethod != smartorder.MethodFuzzy || *r.Line.MatchedProductID != fuzzy {
		t.Fatalf("a failed request disturbed the deterministic outcome: %+v", r.Line)
	}
}

// Progress counts the buyer's rows, not the questions asked. A bar measured in
// questions stops short of the total on any file that repeats a product.
func TestEnhancementProgressCountsLinesNotQuestions(t *testing.T) {
	repo := newEnhanceRepo()
	ai := &stubEnhancer{}

	a := reviewFor(1, "ابليفاى 10مجم", 101)
	b := reviewFor(2, "ابليفاى 10مجم", 101)
	c := reviewFor(3, "ارموويك 50مجم", 103)

	var done int
	e := NewEnhancement(repo, ai, testIndex(), nil)
	e.OnProgress = func(n int) { done = n }
	e.Run(context.Background(), []Review{a, b, c})

	if done != 3 {
		t.Errorf("progress reached %d, want 3", done)
	}
}
