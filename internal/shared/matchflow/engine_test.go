package matchflow_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// fakeAI is a proposal pass and a pair check with scripted answers.
type fakeAI struct {
	mu        sync.Mutex
	propose   func(it matchflow.Item) *matchflow.Decision
	failAbove int
	check     func(p matchflow.Pair) *matchflow.Check
	checkErr  error
	proposals int
	checks    int
	pairs     []matchflow.Pair
}

func (f *fakeAI) Enhance(_ context.Context, b matchflow.Batch) ([]matchflow.Decision, error) {
	f.mu.Lock()
	f.proposals++
	f.mu.Unlock()
	if f.failAbove > 0 && len(b.Items) > f.failAbove {
		return nil, errors.New("truncated")
	}
	var out []matchflow.Decision
	for _, it := range b.Items {
		if d := f.propose(it); d != nil {
			d.Ref = it.Ref
			out = append(out, *d)
		}
	}
	return out, nil
}

func (f *fakeAI) Verify(_ context.Context, b matchflow.CheckBatch) ([]matchflow.Check, error) {
	f.mu.Lock()
	f.checks++
	f.pairs = append(f.pairs, b.Pairs...)
	f.mu.Unlock()
	if f.checkErr != nil {
		return nil, f.checkErr
	}
	var out []matchflow.Check
	for _, p := range b.Pairs {
		if c := f.check(p); c != nil {
			c.Ref = p.Ref
			out = append(out, *c)
		}
	}
	return out, nil
}

// proposalOnly hides the verifier, as a test double without one would.
type proposalOnly struct{ f *fakeAI }

func (p proposalOnly) Enhance(ctx context.Context, b matchflow.Batch) ([]matchflow.Decision, error) {
	return p.f.Enhance(ctx, b)
}

func id(v int64) *int64 { return &v }

func question(key, text string, settled bool, current *int64, window ...int64) matchflow.Question {
	q := matchflow.Question{Key: key, Item: matchflow.Item{
		Text: text, Settled: settled, Loose: settled, CurrentGuess: current}}
	for _, w := range window {
		q.Window = append(q.Window, matchflow.CatalogEntry{ProductID: w, NameAR: "p"})
	}
	return q
}

func same(conf float64) *matchflow.Check {
	return &matchflow.Check{Brand: "same", Strength: "same", Form: "unstated", Variant: "same", Verdict: "same", Confidence: conf}
}

func run(t *testing.T, ai matchflow.Enhancer, qs ...matchflow.Question) map[string]matchflow.Answer {
	t.Helper()
	var mu sync.Mutex
	got := map[string]matchflow.Answer{}
	matchflow.Run(context.Background(), ai, matchflow.Spec{
		Profile:    matchflow.ProfileVendor,
		VerifyPass: true,
		Questions:  qs,
		OnAnswer: func(a matchflow.Answer) {
			mu.Lock()
			got[a.Key] = a
			mu.Unlock()
		},
	})
	return got
}

// The forced match: the list held no right answer, the proposal named the
// nearest thing on it anyway, and the pair check — asked without the list —
// says it is a different product. Nothing is applied.
func TestAForcedMatchIsCaughtByThePairCheck(t *testing.T) {
	ai := &fakeAI{
		propose: func(matchflow.Item) *matchflow.Decision {
			return &matchflow.Decision{ProductID: id(7), Confidence: 0.92}
		},
		check: func(matchflow.Pair) *matchflow.Check {
			return &matchflow.Check{Brand: "same", Strength: "same", Form: "same",
				Variant: "different", Verdict: "different", Confidence: 0.95,
				Reason: "بلس"}
		},
	}
	a := run(t, ai, question("k", "بانادول", false, nil, 7, 8))["k"]
	if a.Decision.ProductID != nil {
		t.Fatalf("applied %d after the pair check said different", *a.Decision.ProductID)
	}
	if a.Proposed == nil || *a.Proposed != 7 {
		t.Errorf("the proposal was lost: %v", a.Proposed)
	}
	if a.Decision.Confidence < matchflow.MinMemoryConfidence {
		t.Errorf("a confident 'different' should be remembered as none (conf %.2f)", a.Decision.Confidence)
	}
}

// Both questions agree: the match stands, at the lower of the two confidences.
func TestAVerifiedProposalIsApplied(t *testing.T) {
	ai := &fakeAI{
		propose: func(matchflow.Item) *matchflow.Decision {
			return &matchflow.Decision{ProductID: id(7), Confidence: 0.97}
		},
		check: func(matchflow.Pair) *matchflow.Check { return same(0.90) },
	}
	a := run(t, ai, question("k", "ابيليفاي", false, nil, 7))["k"]
	if a.Decision.ProductID == nil || *a.Decision.ProductID != 7 || !a.Verified() {
		t.Fatalf("verified proposal not applied: %+v", a)
	}
	if a.Decision.Confidence != 0.90 {
		t.Errorf("confidence %.2f, want the lower of the two (0.90)", a.Decision.Confidence)
	}
	j := matchflow.Judgement{Offered: a.Offered, Verified: a.Verified(),
		Confidence: a.Decision.Confidence, Floor: 0.8}
	if matchflow.Verdict(j, a.Decision.ProductID) != matchflow.OutcomeApply {
		t.Error("a verified answer on an unresolved row should apply")
	}
}

// The pair question is asked without the engine's guess and without the list.
func TestThePairCheckIsAskedWithoutAnchors(t *testing.T) {
	ai := &fakeAI{
		propose: func(matchflow.Item) *matchflow.Decision {
			return &matchflow.Decision{ProductID: id(7), Confidence: 0.95}
		},
		check: func(matchflow.Pair) *matchflow.Check { return same(0.95) },
	}
	run(t, ai, question("k", "x", false, id(8), 7, 8))
	if len(ai.pairs) != 1 {
		t.Fatalf("pairs = %d, want 1", len(ai.pairs))
	}
	p := ai.pairs[0]
	if p.Item.CurrentGuess != nil || len(p.Item.Options) != 0 || p.Item.Settled {
		t.Errorf("the pair check carried an anchor: %+v", p.Item)
	}
	if p.Product.ProductID != 7 {
		t.Errorf("checked product %d, want the proposal 7", p.Product.ProductID)
	}
}

// A model that writes "strength: different" and then "verdict: same" has
// contradicted itself about a medicine. That is not an answer.
func TestASelfContradictingCheckIsNotAccepted(t *testing.T) {
	c := same(0.99)
	c.Strength = "different"
	if c.Accepts(0.8) {
		t.Fatal("accepted a 'same' whose strength was 'different'")
	}
	c = same(0.99)
	c.Brand = "unclear"
	if c.Accepts(0.8) {
		t.Fatal("accepted a 'same' with an unclear brand")
	}
	if !same(0.85).Accepts(0.8) {
		t.Fatal("refused a clean 'same'")
	}
	if same(0.75).Accepts(0.8) {
		t.Fatal("accepted a 'same' below the floor")
	}
}

// A dispute the pair check contradicts was the list talking. The engine's own
// match is confirmed and nobody is asked.
func TestADisputeTheCheckContradictsIsAConfirmation(t *testing.T) {
	ai := &fakeAI{
		propose: func(matchflow.Item) *matchflow.Decision {
			return &matchflow.Decision{ProductID: id(8), Confidence: 0.85}
		},
		check: func(p matchflow.Pair) *matchflow.Check {
			if p.Product.ProductID == 5 {
				return same(0.96)
			}
			return nil
		},
	}
	a := run(t, ai, question("k", "x", true, id(5), 5, 8))["k"]
	if a.Decision.ProductID == nil || *a.Decision.ProductID != 5 {
		t.Fatalf("answer %v, want the engine's own 5 confirmed", a.Decision.ProductID)
	}
	j := matchflow.Judgement{Settled: true, Current: id(5), Offered: a.Offered,
		Confidence: a.Decision.Confidence, Floor: 0.8, Verified: a.Verified()}
	if v := matchflow.Verdict(j, a.Decision.ProductID); v != matchflow.OutcomeKeep {
		t.Errorf("verdict %v, want keep", v)
	}
}

// And a dispute the check upholds goes to a person.
func TestADisputeTheCheckUpholdsGoesToReview(t *testing.T) {
	ai := &fakeAI{
		propose: func(matchflow.Item) *matchflow.Decision {
			return &matchflow.Decision{ProductID: id(8), Confidence: 0.9}
		},
		check: func(matchflow.Pair) *matchflow.Check {
			return &matchflow.Check{Brand: "same", Strength: "different", Form: "same",
				Variant: "same", Verdict: "different", Confidence: 0.95}
		},
	}
	a := run(t, ai, question("k", "x", true, id(5), 5, 8))["k"]
	j := matchflow.Judgement{Settled: true, Current: id(5), Offered: a.Offered,
		Confidence: a.Decision.Confidence, Floor: 0.8, Verified: a.Verified()}
	if v := matchflow.Verdict(j, a.Decision.ProductID); v != matchflow.OutcomeReview {
		t.Errorf("verdict %v, want review", v)
	}
}

// When the pair check cannot be asked, an unresolved row where the model names
// a product the engine has no opinion on is still applied — the pair check
// exists to catch forced matches (the list bias), not to veto a model that
// independently found something the engine could not.
func TestAnUnverifiableProposalOnAnUnresolvedRowIsApplied(t *testing.T) {
	ai := &fakeAI{
		propose: func(matchflow.Item) *matchflow.Decision {
			return &matchflow.Decision{ProductID: id(7), Confidence: 0.99}
		},
		checkErr: errors.New("gateway down"),
	}
	a := run(t, ai, question("k", "x", false, nil, 7))["k"]
	if a.Decision.ProductID == nil || *a.Decision.ProductID != 7 {
		t.Fatalf("unverified proposal discarded: %v, want 7", a.Decision.ProductID)
	}
	j := matchflow.Judgement{Offered: a.Offered, Verified: a.Verified(),
		Confidence: a.Decision.Confidence, Floor: 0.8}
	if v := matchflow.Verdict(j, a.Decision.ProductID); v != matchflow.OutcomeApply {
		t.Errorf("verdict %v, want apply for an unresolved row", v)
	}
}

// But when the model agrees with the engine's own guess, an unverified
// agreement is not enough — OutcomeApply requires Verified=true for that path,
// because two methods agreeing is only meaningful when verified independently.
func TestAnUnverifiableConfirmationIsNotApplied(t *testing.T) {
	ai := &fakeAI{
		propose: func(matchflow.Item) *matchflow.Decision {
			return &matchflow.Decision{ProductID: id(7), Confidence: 0.99}
		},
		checkErr: errors.New("gateway down"),
	}
	a := run(t, ai, question("k", "x", false, id(7), 7))["k"]
	j := matchflow.Judgement{Current: id(7), Offered: a.Offered, Verified: a.Verified(),
		Confidence: a.Decision.Confidence, Floor: 0.8}
	if v := matchflow.Verdict(j, a.Decision.ProductID); v != matchflow.OutcomeKeep {
		t.Errorf("verdict %v, want keep for an unverified confirmation", v)
	}
}

// A batch that fails is halved until it answers, rather than dropped.
func TestAFailedBatchIsSplitNotDropped(t *testing.T) {
	ai := &fakeAI{
		failAbove: 5,
		propose: func(matchflow.Item) *matchflow.Decision {
			return &matchflow.Decision{Confidence: 0.9}
		},
	}
	var qs []matchflow.Question
	for i := 0; i < 20; i++ {
		qs = append(qs, question(string(rune('a'+i)), "x", false, nil, int64(i+1)))
	}
	got := run(t, proposalOnly{ai}, qs...)
	if len(got) != 20 {
		t.Fatalf("answered %d of 20 after a failure; a failed batch must be split", len(got))
	}
}

// An answer that ran out of room — some refs answered, some not — is finished
// by asking again for what was skipped.
func TestSkippedItemsAreAskedAgain(t *testing.T) {
	seen := map[string]int{}
	var mu sync.Mutex
	ai := &fakeAI{propose: func(it matchflow.Item) *matchflow.Decision {
		mu.Lock()
		defer mu.Unlock()
		seen[it.Text]++
		if it.Text == "late" && seen[it.Text] == 1 {
			return nil
		}
		return &matchflow.Decision{Confidence: 0.9}
	}}
	got := run(t, proposalOnly{ai},
		question("a", "early", false, nil, 1),
		question("b", "late", false, nil, 2))
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) != 2 {
		t.Fatalf("answered %v, want both", keys)
	}
}

// A proposal the identity guard will refuse is not paid for twice.
func TestAScreenedProposalIsNotChecked(t *testing.T) {
	ai := &fakeAI{
		propose: func(matchflow.Item) *matchflow.Decision {
			return &matchflow.Decision{ProductID: id(7), Confidence: 0.95}
		},
		check: func(matchflow.Pair) *matchflow.Check { return same(0.95) },
	}
	matchflow.Run(context.Background(), ai, matchflow.Spec{
		Profile:   matchflow.ProfileVendor,
		Questions: []matchflow.Question{question("k", "x", false, nil, 7)},
		Screen:    func(string, int64) bool { return false },
	})
	if ai.checks != 0 {
		t.Errorf("checked a proposal the guard refuses (%d checks)", ai.checks)
	}
}

// Attribution reaches every request, proposal and check alike. A background
// job has no actor on its context, and an unattributed request is one nobody
// can bill or cap.
func TestEveryRequestIsAttributed(t *testing.T) {
	var batches []matchflow.Batch
	var checks []matchflow.CheckBatch
	var mu sync.Mutex
	ai := &attributionSpy{
		enhance: func(b matchflow.Batch) {
			mu.Lock()
			batches = append(batches, b)
			mu.Unlock()
		},
		verify: func(b matchflow.CheckBatch) {
			mu.Lock()
			checks = append(checks, b)
			mu.Unlock()
		},
	}
	matchflow.Run(context.Background(), ai, matchflow.Spec{
		Profile:        matchflow.ProfileOrder,
		VerifyPass:     true,
		Questions:      []matchflow.Question{question("k", "x", false, nil, 7)},
		Feature:        matchflow.FeatureSmartOrder,
		OrganizationID: 42,
		UserID:         9,
	})
	if len(batches) == 0 || len(checks) == 0 {
		t.Fatalf("proposals=%d checks=%d, want both", len(batches), len(checks))
	}
	for _, b := range batches {
		if b.OrganizationID != 42 || b.UserID != 9 || b.Feature != matchflow.FeatureSmartOrder {
			t.Errorf("proposal unattributed: org=%d user=%d feature=%q", b.OrganizationID, b.UserID, b.Feature)
		}
	}
	for _, c := range checks {
		if c.OrganizationID != 42 || c.UserID != 9 || c.Feature != matchflow.FeatureSmartOrder {
			t.Errorf("check unattributed: org=%d user=%d feature=%q", c.OrganizationID, c.UserID, c.Feature)
		}
	}
}

type attributionSpy struct {
	enhance func(matchflow.Batch)
	verify  func(matchflow.CheckBatch)
}

func (s *attributionSpy) Enhance(_ context.Context, b matchflow.Batch) ([]matchflow.Decision, error) {
	s.enhance(b)
	out := make([]matchflow.Decision, len(b.Items))
	for i, it := range b.Items {
		out[i] = matchflow.Decision{Ref: it.Ref, ProductID: id(7), Confidence: 0.95}
	}
	return out, nil
}
func (s *attributionSpy) Verify(_ context.Context, b matchflow.CheckBatch) ([]matchflow.Check, error) {
	s.verify(b)
	out := make([]matchflow.Check, len(b.Pairs))
	for i, p := range b.Pairs {
		c := same(0.95)
		c.Ref = p.Ref
		out[i] = *c
	}
	return out, nil
}
