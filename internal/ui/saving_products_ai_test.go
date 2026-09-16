package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// fakeEnhancer stands in for the Gateway.
//
// It records what it was asked, so the tests can assert the shape of the request
// as well as the effect of the answer — a stage that sends the wrong window is
// broken even when its answers happen to land.
type fakeEnhancer struct {
	calls  int
	last   matchflow.Batch
	answer func(matchflow.Batch) ([]matchflow.Decision, error)
}

func (f *fakeEnhancer) Enhance(_ context.Context, b matchflow.Batch) ([]matchflow.Decision, error) {
	f.calls++
	f.last = b
	if f.answer == nil {
		return nil, nil
	}
	return f.answer(b)
}

func savingTestEngine() *SavingProductMatchEngine {
	return NewSavingProductMatchEngine([]catalog.MatchProduct{
		{ID: 501, SKU: "AUG-1G", NameAR: "أوجمنتين 1 جم 14 قرص", NameEN: "Augmentin 1g 14 Tablets",
			DosageForm: "أقراص", Concentration: "1g"},
		{ID: 502, SKU: "AUG-625", NameAR: "أوجمنتين 625 مجم 14 قرص", NameEN: "Augmentin 625mg 14 Tablets",
			DosageForm: "أقراص", Concentration: "625mg"},
		{ID: 503, SKU: "PAN-24", NameAR: "بانادول اكسترا 24 قرص", NameEN: "Panadol Extra 24 Tablets",
			DosageForm: "أقراص"},
	})
}

// TestSavingAILinksAnAnswerItCanVerify is the end-to-end proof that the stage is
// wired: an unlinked row goes in, the model names a product from the window, and
// the row comes back linked with the catalogue's own name on it.
func TestSavingAILinksAnAnswerItCanVerify(t *testing.T) {
	engine := savingTestEngine()
	items := []*StagedSavingItem{
		{Index: 1, NameProduct: "اوجمنتين 1جم اقراص", MatchType: "unlinked"},
	}

	ai := &fakeEnhancer{answer: func(b matchflow.Batch) ([]matchflow.Decision, error) {
		id := int64(501)
		return []matchflow.Decision{{Ref: b.Items[0].Ref, ProductID: &id, Confidence: 0.95}}, nil
	}}

	if got := enhanceSavingItems(context.Background(), ai, nil, engine, items, nil); got != 1 {
		t.Fatalf("improved = %d, want 1", got)
	}
	if ai.calls != 1 {
		t.Fatalf("gateway calls = %d, want 1", ai.calls)
	}
	if items[0].ProductID == nil || *items[0].ProductID != 501 {
		t.Fatalf("row not linked: %v", items[0].ProductID)
	}
	if items[0].MatchType != "ai" {
		t.Errorf("match type = %q, want ai", items[0].MatchType)
	}
	if items[0].MasterProductName == "" {
		t.Error("the catalogue name was not written back; the review table would show only an id")
	}

	// The window must actually carry the candidates, and every item must be
	// able to name any of them.
	if len(ai.last.Catalog) == 0 {
		t.Fatal("the request carried no catalogue window")
	}
	if len(ai.last.Items) != 1 || ai.last.Items[0].Text == "" {
		t.Fatalf("the request carried no usable item: %+v", ai.last.Items)
	}
}

// TestSavingAIRefusesAnAnswerTheCatalogueContradicts is the guard that matters
// more than the matches. The model answering 625 mg for a row that says 1 g is
// exactly the mistake this list used to be capable of, and the prompt saying not
// to is a tendency rather than a guarantee.
func TestSavingAIRefusesAnAnswerTheCatalogueContradicts(t *testing.T) {
	engine := savingTestEngine()
	items := []*StagedSavingItem{
		{Index: 1, NameProduct: "اوجمنتين 1جم اقراص", MatchType: "unlinked"},
	}

	ai := &fakeEnhancer{answer: func(b matchflow.Batch) ([]matchflow.Decision, error) {
		wrongStrength := int64(502)
		return []matchflow.Decision{{Ref: b.Items[0].Ref, ProductID: &wrongStrength, Confidence: 0.99}}, nil
	}}

	if got := enhanceSavingItems(context.Background(), ai, nil, engine, items, nil); got != 0 {
		t.Fatalf("improved = %d, want 0 — a contradicted answer must be refused", got)
	}
	if items[0].ProductID != nil {
		t.Fatalf("row was linked to the wrong strength: %v", *items[0].ProductID)
	}
}

// TestSavingAIRefusesAProductItWasNotOffered guards against an invented id
// becoming a link to a real product.
func TestSavingAIRefusesAProductItWasNotOffered(t *testing.T) {
	engine := savingTestEngine()
	items := []*StagedSavingItem{{Index: 1, NameProduct: "اوجمنتين 1جم اقراص", MatchType: "unlinked"}}

	ai := &fakeEnhancer{answer: func(b matchflow.Batch) ([]matchflow.Decision, error) {
		invented := int64(999999)
		return []matchflow.Decision{{Ref: b.Items[0].Ref, ProductID: &invented, Confidence: 1.0}}, nil
	}}

	if got := enhanceSavingItems(context.Background(), ai, nil, engine, items, nil); got != 0 {
		t.Fatalf("improved = %d, want 0", got)
	}
}

// TestSavingAIRefusesLowConfidence enforces the floor the prompt only asks for.
func TestSavingAIRefusesLowConfidence(t *testing.T) {
	engine := savingTestEngine()
	items := []*StagedSavingItem{{Index: 1, NameProduct: "اوجمنتين 1جم اقراص", MatchType: "unlinked"}}

	ai := &fakeEnhancer{answer: func(b matchflow.Batch) ([]matchflow.Decision, error) {
		id := int64(501)
		return []matchflow.Decision{{Ref: b.Items[0].Ref, ProductID: &id, Confidence: 0.4}}, nil
	}}

	if got := enhanceSavingItems(context.Background(), ai, nil, engine, items, nil); got != 0 {
		t.Fatalf("improved = %d, want 0", got)
	}
}

// TestSavingAISkipsRowsTheCatalogueCannotAnswer is the plausibility gate: a row
// with nothing like it in the catalogue is never sent, so the stage costs
// nothing on a file of products the platform does not carry.
func TestSavingAISkipsRowsTheCatalogueCannotAnswer(t *testing.T) {
	engine := savingTestEngine()
	items := []*StagedSavingItem{
		{Index: 1, NameProduct: "مفك براغي فيليبس مقاس 6", MatchType: "unlinked"},
	}

	ai := &fakeEnhancer{}
	if got := enhanceSavingItems(context.Background(), ai, nil, engine, items, nil); got != 0 {
		t.Fatalf("improved = %d, want 0", got)
	}
	if ai.calls != 0 {
		t.Errorf("the gateway was called %d times for a row it cannot answer", ai.calls)
	}
}

// 🚦 A row the deterministic engine linked IS checked, and a model that
// disagrees may not quietly swap the product for another one.
//
// The rule used to be that a settled row was never sent at all, which sounds
// safe and is not: it meant the engine's confident mistakes were the one class
// of error in the file nobody ever looked at twice, and a pharmacy has no way
// to see a wrong link that arrived with 100% beside it.
//
// So the row is sent, and the disagreement is resolved the only honest way. Two
// independent methods each named a product and they named different ones;
// neither wins. The link is withdrawn and the row goes to the review screen,
// where the person whose list it is can settle it.
func TestSavingAIChecksLinkedRowsAndWithdrawsOnDisagreement(t *testing.T) {
	engine := savingTestEngine()
	settled := int64(503)
	items := []*StagedSavingItem{
		{Index: 1, NameProduct: "بانادول اكسترا 24 قرص", ProductID: &settled, MatchType: "exact_name"},
	}

	// The model rejects the link: none of the products it was shown is this
	// row. That is the canonical disagreement for a check, and the one a
	// pharmacy most needs to see.
	ai := &fakeEnhancer{answer: func(b matchflow.Batch) ([]matchflow.Decision, error) {
		out := make([]matchflow.Decision, 0, len(b.Items))
		for _, it := range b.Items {
			if !it.Settled {
				t.Errorf("a linked row was sent as a resolution rather than a check")
			}
			out = append(out, matchflow.Decision{Ref: it.Ref, Confidence: 0.95})
		}
		return out, nil
	}}

	_ = enhanceSavingItems(context.Background(), ai, nil, engine, items, nil)
	if ai.calls == 0 {
		t.Fatalf("a linked row was never checked")
	}
	if items[0].ProductID != nil {
		t.Errorf("the model's product was applied over the engine's: %d", *items[0].ProductID)
	}
	if items[0].MatchType != savingMatchTypeDisputed {
		t.Errorf("match type = %q, want %q — the disagreement is not visible on the screen",
			items[0].MatchType, savingMatchTypeDisputed)
	}
}

// A row whose link came from an identifier is not checked at all.
//
// A barcode is the same physical package and an id the file stated outright is
// the pharmacy's own assertion. A model cannot improve on either, and asking
// spends the budget the uncertain rows need.
func TestSavingAILeavesIdentifierLinksAlone(t *testing.T) {
	engine := savingTestEngine()
	settled := int64(503)
	items := []*StagedSavingItem{
		{Index: 1, NameProduct: "بانادول اكسترا 24 قرص", ProductID: &settled, MatchType: "barcode"},
	}

	ai := &fakeEnhancer{}
	_ = enhanceSavingItems(context.Background(), ai, nil, engine, items, nil)
	if ai.calls != 0 {
		t.Errorf("a barcode link was sent for adjudication")
	}
	if items[0].ProductID == nil || *items[0].ProductID != settled {
		t.Errorf("a barcode link was disturbed")
	}
}

// TestSavingAIDegradesWhenTheGatewayFails is AGENTS.md R3: a pharmacy must be
// able to build its list when the Gateway is down.
func TestSavingAIDegradesWhenTheGatewayFails(t *testing.T) {
	engine := savingTestEngine()
	items := []*StagedSavingItem{{Index: 1, NameProduct: "اوجمنتين 1جم اقراص", MatchType: "unlinked"}}

	ai := &fakeEnhancer{answer: func(matchflow.Batch) ([]matchflow.Decision, error) {
		return nil, errors.New("gateway unavailable")
	}}

	if got := enhanceSavingItems(context.Background(), ai, nil, engine, items, nil); got != 0 {
		t.Fatalf("improved = %d, want 0", got)
	}
	if items[0].MatchType != "unlinked" {
		t.Errorf("the row did not keep its deterministic outcome: %q", items[0].MatchType)
	}
}

// TestSavingAIIsOffByDefault: the toggle is the consent, and every staging path
// goes through this one function so none of them can forget to ask.
func TestSavingAIIsOffByDefault(t *testing.T) {
	h := &UIHandler{}
	ai := &fakeEnhancer{answer: func(b matchflow.Batch) ([]matchflow.Decision, error) {
		id := int64(501)
		return []matchflow.Decision{{Ref: b.Items[0].Ref, ProductID: &id, Confidence: 1.0}}, nil
	}}
	h.SetMatchEnhancer(ai)

	items := []*StagedSavingItem{{Index: 1, NameProduct: "اوجمنتين 1جم اقراص", MatchType: "unlinked"}}
	if got := h.enhanceSaving(context.Background(), false, savingTestEngine(), items); got != 0 {
		t.Fatalf("the stage ran with the toggle off, improving %d", got)
	}
	if ai.calls != 0 {
		t.Errorf("the gateway was called with the toggle off")
	}

	if got := h.enhanceSaving(context.Background(), true, savingTestEngine(), items); got != 1 {
		t.Fatalf("the stage did not run with the toggle on, improving %d", got)
	}
}

// TestSavingAIWithoutAnEnhancerIsANoOp covers the deployment where the Gateway
// was never wired at all.
func TestSavingAIWithoutAnEnhancerIsANoOp(t *testing.T) {
	h := &UIHandler{}
	items := []*StagedSavingItem{{Index: 1, NameProduct: "اوجمنتين 1جم اقراص", MatchType: "unlinked"}}
	if got := h.enhanceSaving(context.Background(), true, savingTestEngine(), items); got != 0 {
		t.Fatalf("improved = %d with no enhancer wired", got)
	}
}

type fakeMemory struct {
	lookups    int
	known      map[string]matchflow.Remembered
	autoAnswer func(k string) (matchflow.Remembered, bool)
}

func (m *fakeMemory) Lookup(_ context.Context, keys []string) (map[string]matchflow.Remembered, error) {
	m.lookups++
	out := make(map[string]matchflow.Remembered)
	for _, k := range keys {
		if m.autoAnswer != nil {
			if ans, ok := m.autoAnswer(k); ok {
				out[k] = ans
				continue
			}
		}
		if r, ok := m.known[k]; ok {
			out[k] = r
		}
	}
	return out, nil
}

func (m *fakeMemory) Save(context.Context, []matchflow.Remembered) error {
	return nil
}

func (m *fakeMemory) SaveAlias(context.Context, int64, string, string, float64) error {
	return nil
}

// TestSavingAIMemoryPreventsGatewayCall verifies that a pre-seeded cached decision
// fulfills the match without invoking the Gateway enhancer (0 calls).
func TestSavingAIMemoryPreventsGatewayCall(t *testing.T) {
	engine := savingTestEngine()
	items := []*StagedSavingItem{
		{Index: 1, NameProduct: "اوجمنتين 1جم اقراص", MatchType: "unlinked"},
	}

	ai := &fakeEnhancer{}
	prodID := int64(501)
	mem := &fakeMemory{
		autoAnswer: func(k string) (matchflow.Remembered, bool) {
			return matchflow.Remembered{
				Key:             k,
				NormName:        "اوجمنتين 1جم اقراص",
				ChosenProductID: &prodID,
				Confidence:      0.95,
				Scope:           "platform",
				Source:          "admin",
			}, true
		},
	}

	got := enhanceSavingItems(context.Background(), ai, mem, engine, items, nil)
	if got != 1 {
		t.Fatalf("improved = %d, want 1", got)
	}
	if mem.lookups == 0 {
		t.Errorf("decision memory was never looked up")
	}
	if ai.calls != 0 {
		t.Fatalf("gateway calls = %d, want 0 (cached decision must prevent gateway calls)", ai.calls)
	}
	if items[0].ProductID == nil || *items[0].ProductID != prodID {
		t.Fatalf("expected item linked to %d, got %v", prodID, items[0].ProductID)
	}
	if items[0].MatchType != "ai" {
		t.Errorf("match type = %q, want 'ai'", items[0].MatchType)
	}
}

// TestSavingAIUpdatesAllDuplicateTargets ensures that if multiple rows in the uploaded
// spreadsheet share the same product name, all targets receive the AI match verdict.
func TestSavingAIUpdatesAllDuplicateTargets(t *testing.T) {
	engine := savingTestEngine()
	items := []*StagedSavingItem{
		{Index: 1, NameProduct: "اوجمنتين 1جم اقراص", MatchType: "unlinked"},
		{Index: 2, NameProduct: "اوجمنتين 1جم اقراص", MatchType: "unlinked"},
	}

	ai := &fakeEnhancer{answer: func(b matchflow.Batch) ([]matchflow.Decision, error) {
		id := int64(501)
		return []matchflow.Decision{{Ref: b.Items[0].Ref, ProductID: &id, Confidence: 0.95}}, nil
	}}

	if got := enhanceSavingItems(context.Background(), ai, nil, engine, items, nil); got != 2 {
		t.Fatalf("improved = %d, want 2", got)
	}
	for i, item := range items {
		if item.ProductID == nil || *item.ProductID != 501 {
			t.Errorf("row %d productID = %v, want 501", i+1, item.ProductID)
		}
		if item.MatchType != "ai" {
			t.Errorf("row %d matchType = %q, want 'ai'", i+1, item.MatchType)
		}
	}
}

// TestSavingAIProgressCallback verifies that enhanceSavingItemsWithProgress fires progress callbacks.
func TestSavingAIProgressCallback(t *testing.T) {
	engine := savingTestEngine()
	items := []*StagedSavingItem{
		{Index: 1, NameProduct: "اوجمنتين 1جم اقراص", MatchType: "unlinked"},
	}

	ai := &fakeEnhancer{answer: func(b matchflow.Batch) ([]matchflow.Decision, error) {
		id := int64(501)
		return []matchflow.Decision{{Ref: b.Items[0].Ref, ProductID: &id, Confidence: 0.95}}, nil
	}}

	var progressReports []int
	onProgress := func(done, total, improved int) {
		progressReports = append(progressReports, done)
	}

	enhanceSavingItemsWithProgress(context.Background(), ai, nil, engine, items, nil, onProgress)
	if len(progressReports) < 2 {
		t.Fatalf("expected at least initial and completion progress calls, got %d", len(progressReports))
	}
}
