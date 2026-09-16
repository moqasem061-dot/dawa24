package catalog_test

import (
	"context"
	"errors"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// The similarity tier is why the master catalogue import stopped duplicating
// itself. Exact identifiers settle a re-upload of the same file and little
// else: a supplier writes "بروفين 400 مج اقراص" where the catalogue holds
// "بروفين 400مجم أقراص", and under exact matching alone every one of those
// became a second catalogue entry for a product already on file.

const similarFixture = "اسم الصنف,كود الصنف,سعر البيع\n" +
	"بروفين 400 مج اقراص,SUP-9,78.00\n"

func catalogueOf(products ...catalog.MatchProduct) []catalog.MatchProduct { return products }

func TestSimilarityTierMatchesADifferentSpelling(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = catalogueOf(catalog.MatchProduct{
		ID: 4242, NameAR: "بروفين 400مجم أقراص", DosageForm: "أقراص", Concentration: "400 مجم",
	})
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, err := svc.AnalyzeImport(ctx, []byte(similarFixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if len(store.rows) != 1 {
		t.Fatalf("staged %d rows, want 1", len(store.rows))
	}
	row := store.rows[0]
	if row.MatchedProductID == nil {
		t.Fatalf("row was not matched; it would have duplicated product 4242")
	}
	if *row.MatchedProductID != 4242 {
		t.Errorf("matched product %d, want 4242", *row.MatchedProductID)
	}
	if row.MatchReason != catalog.MatchSimilar {
		t.Errorf("match reason = %q, want %q", row.MatchReason, catalog.MatchSimilar)
	}
	if row.Action != catalog.ActionUpdate {
		t.Errorf("action = %q, want update", row.Action)
	}
}

// A different strength is a different medicine. The similarity tier must not
// tie them together however similar the names are, because the consequence is
// the wrong product's price and pack in the shared catalogue.
func TestSimilarityTierRefusesADifferentStrength(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = catalogueOf(catalog.MatchProduct{
		ID: 4243, NameAR: "بروفين 600مجم أقراص", DosageForm: "أقراص", Concentration: "600 مجم",
	})
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, err := svc.AnalyzeImport(ctx, []byte(similarFixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if store.rows[0].MatchedProductID != nil {
		t.Fatalf("400 mg was matched to a 600 mg product; that is the wrong medicine")
	}
	if store.rows[0].Action != catalog.ActionInsert {
		t.Errorf("action = %q, want insert", store.rows[0].Action)
	}
}

// The AI tier's band: plausible, but not settled.
//
// The imported row names a sustained-release variant the catalogue entry does
// not — same brand, same strength, same form, one distinguishing word missing
// on one side. That is exactly the case a deterministic engine should refuse to
// decide and a model can often settle, and it is the only band the AI tier is
// ever given.
const uncertainFixture = "اسم الصنف,كود الصنف,سعر البيع\n" +
	"بروفين ريتارد 400 مج اقراص,SUP-9,78.00\n"

func uncertainCatalogue() []catalog.MatchProduct {
	return catalogueOf(catalog.MatchProduct{
		ID: 4244, NameAR: "بروفين 400مجم أقراص", DosageForm: "أقراص", Concentration: "400 مجم",
	})
}

// stubAdjudicator answers with whatever it is scripted to.
type stubAdjudicator struct {
	calls   int
	batches []int
	answer  func(req catalog.MatchAdjudicationRequest) ([]catalog.MatchAdjudicationResult, error)
}

func (s *stubAdjudicator) AdjudicateMatches(
	_ context.Context, req catalog.MatchAdjudicationRequest,
) ([]catalog.MatchAdjudicationResult, error) {
	s.calls++
	s.batches = append(s.batches, len(req.Items))
	if s.answer != nil {
		return s.answer(req)
	}
	return nil, nil
}

// The AI tier may only choose among the candidates it was given. An id it was
// not offered is a hallucination, and applying one would tie a supplier's price
// to a product nobody proposed.
func TestAdjudicationRejectsAProductItWasNotOffered(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = uncertainCatalogue()
	svc, _ := newImportService(t, store)

	invented := int64(999999)
	ai := &stubAdjudicator{answer: func(req catalog.MatchAdjudicationRequest) ([]catalog.MatchAdjudicationResult, error) {
		out := make([]catalog.MatchAdjudicationResult, 0, len(req.Items))
		for _, it := range req.Items {
			out = append(out, catalog.MatchAdjudicationResult{
				Ref: it.Ref, ProductID: &invented, Confidence: 0.99,
			})
		}
		return out, nil
	}}
	svc.SetMatchAdjudicator(ai)

	row := prepareWithAI(t, svc, store)
	if row.MatchedProductID != nil {
		t.Fatalf("an id that was never a candidate was applied: %d", *row.MatchedProductID)
	}
}

// A gateway failure must leave the import working. The deterministic outcome —
// "this is a new product" — is complete and usable on its own.
func TestAdjudicationFailureLeavesTheImportUsable(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = uncertainCatalogue()
	svc, _ := newImportService(t, store)

	ai := &stubAdjudicator{answer: func(catalog.MatchAdjudicationRequest) ([]catalog.MatchAdjudicationResult, error) {
		return nil, errors.New("gateway unavailable")
	}}
	svc.SetMatchAdjudicator(ai)

	row := prepareWithAI(t, svc, store)
	if row.Action != catalog.ActionInsert {
		t.Fatalf("action = %q, want insert; a failed model must not fail the import", row.Action)
	}
	if ai.calls == 0 {
		t.Error("the adjudicator was never called")
	}
}

// A low-confidence answer is not applied. The model saying "probably" about a
// pharmaceutical match is not enough to overwrite a catalogue entry.
func TestAdjudicationIgnoresLowConfidence(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = uncertainCatalogue()
	svc, _ := newImportService(t, store)

	chosen := int64(4244)
	svc.SetMatchAdjudicator(&stubAdjudicator{
		answer: func(req catalog.MatchAdjudicationRequest) ([]catalog.MatchAdjudicationResult, error) {
			out := make([]catalog.MatchAdjudicationResult, 0, len(req.Items))
			for _, it := range req.Items {
				out = append(out, catalog.MatchAdjudicationResult{
					Ref: it.Ref, ProductID: &chosen, Confidence: 0.4,
				})
			}
			return out, nil
		},
	})

	if row := prepareWithAI(t, svc, store); row.MatchedProductID != nil {
		t.Fatalf("a 0.4-confidence answer was applied")
	}
}

// With AI switched off the tier must not be reached at all, whatever is wired.
func TestAdjudicationIsSkippedWhenTheSwitchIsOff(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = uncertainCatalogue()
	svc, _ := newImportService(t, store)
	ai := &stubAdjudicator{}
	svc.SetMatchAdjudicator(ai)

	ctx := context.Background()
	session, _, err := svc.AnalyzeImport(ctx, []byte(uncertainFixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}
	opts := catalog.DefaultImportOptions()
	opts.UseAI = false
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: opts,
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if ai.calls != 0 {
		t.Fatalf("the adjudicator was called %d times with AI switched off", ai.calls)
	}
}

// prepareWithAI runs one preparation with the AI switch on and returns the
// single staged row.
func prepareWithAI(t *testing.T, svc *catalog.Service, store *memoryImportStore) *catalog.StagingRow {
	t.Helper()
	ctx := context.Background()
	session, _, err := svc.AnalyzeImport(ctx, []byte(uncertainFixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}
	opts := catalog.DefaultImportOptions()
	opts.UseAI = true
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: opts,
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if len(store.rows) != 1 {
		t.Fatalf("staged %d rows, want 1", len(store.rows))
	}
	return store.rows[0]
}

type stubMatchMemory struct {
	known      map[string]matchflow.Remembered
	autoAnswer func(key string) (matchflow.Remembered, bool)
}

func (m *stubMatchMemory) Lookup(_ context.Context, keys []string) (map[string]matchflow.Remembered, error) {
	out := make(map[string]matchflow.Remembered)
	for _, k := range keys {
		if r, ok := m.known[k]; ok {
			out[k] = r
		} else if m.autoAnswer != nil {
			if ans, matched := m.autoAnswer(k); matched {
				out[k] = ans
			}
		}
	}
	return out, nil
}

func (m *stubMatchMemory) Save(context.Context, []matchflow.Remembered) error {
	return nil
}

func (m *stubMatchMemory) SaveAlias(context.Context, int64, string, string, float64) error {
	return nil
}

// A pre-seeded decision in match memory is applied without calling the adjudicator.
func TestImportMatchMemoryPreventsAdjudicatorCall(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = catalogueOf(catalog.MatchProduct{
		ID: 4244, NameAR: "بروفين ريتارد 400مجم أقراص", DosageForm: "أقراص", Concentration: "400 مجم",
	})
	svc, _ := newImportService(t, store)

	ai := &stubAdjudicator{}
	svc.SetMatchAdjudicator(ai)

	var memLookups int
	chosen := int64(4244)
	mem := &stubMatchMemory{
		autoAnswer: func(k string) (matchflow.Remembered, bool) {
			memLookups++
			return matchflow.Remembered{
				Key:             k,
				NormName:        "بروفين ريتارد 400 مج اقراص",
				ChosenProductID: &chosen,
				Confidence:      0.95,
				Scope:           "platform",
				Source:          "admin",
			}, true
		},
	}
	svc.SetMatchMemory(mem)

	ctx := context.Background()
	fixture := "اسم الصنف,كود الصنف,سعر البيع\n" +
		"بروفين ريتارد 400 مج اقراص,SUP-9,78.00\n"
	session, _, err := svc.AnalyzeImport(ctx, []byte(fixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}
	opts := catalog.DefaultImportOptions()
	opts.UseAI = true
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: opts,
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if len(store.rows) != 1 {
		t.Fatalf("staged %d rows, want 1", len(store.rows))
	}
	row := store.rows[0]

	if memLookups == 0 {
		t.Errorf("match memory was never consulted")
	}
	if ai.calls != 0 {
		t.Fatalf("adjudicator was called %d times; expected 0 due to memory cache hit", ai.calls)
	}
	if row.MatchedProductID == nil || *row.MatchedProductID != chosen {
		t.Fatalf("row not matched to %d: %v", chosen, row.MatchedProductID)
	}
}
