package catalog_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// A switch that does nothing is worse than no switch, because it is believed.
//
// Every AI port the import can reach is recorded here rather than stubbed
// silently, so "AI is off" is a property the test can assert instead of a claim
// the code makes in four separate places. It used to be exactly that: four
// reads of session.Options.UseAI, two of which were reached only because a
// slice happened to be empty when the switch was off.

type recordingAI struct {
	columns     int
	values      int
	adjudicated int
	recalled    int
	remembered  int
}

func (r *recordingAI) MapColumns(context.Context, catalog.ColumnMapRequest) (catalog.ColumnMapResult, error) {
	r.columns++
	return catalog.ColumnMapResult{}, nil
}

func (r *recordingAI) MapValues(context.Context, catalog.ValueMapRequest) (catalog.ValueMapResult, error) {
	r.values++
	return catalog.ValueMapResult{}, nil
}

func (r *recordingAI) Available(context.Context) bool { return true }

func (r *recordingAI) AdjudicateMatches(
	context.Context, catalog.MatchAdjudicationRequest,
) ([]catalog.MatchAdjudicationResult, error) {
	r.adjudicated++
	return nil, nil
}

func (r *recordingAI) Lookup(context.Context, []string) (map[string]matchflow.Remembered, error) {
	r.remembered++
	return nil, nil
}

func (r *recordingAI) Save(context.Context, []matchflow.Remembered) error { return nil }

func (r *recordingAI) SaveAlias(context.Context, int64, string, string, float64) error { return nil }

func (r *recordingAI) touched() int {
	return r.columns + r.values + r.adjudicated + r.recalled + r.remembered
}

// populatedCatalogue is the condition the failures were reported under: an
// import into a catalogue that already holds products, which is when the
// similarity and AI tiers run at all.
func populatedCatalogue(store *memoryImportStore) {
	store.catalogue = []catalog.MatchProduct{
		{ID: 1, NameAR: "بانادول اكسترا", Concentration: "500 مجم", DosageForm: "اقراص"},
		{ID: 2, NameAR: "كتافلام", Concentration: "50 مجم", DosageForm: "اقراص"},
		{ID: 3, NameAR: "اوجمنتين", Concentration: "1 جم", DosageForm: "حقن"},
	}
	store.vocab = catalog.EnrichVocabulary{
		Categories:  []catalog.TaxonomyOption{{ID: 9, Name: "مسكنات"}},
		DosageForms: []string{"اقراص", "حقن"},
	}
}

func TestImportTouchesNoAIPortWhenTheSwitchIsOff(t *testing.T) {
	store := newMemoryStore()
	populatedCatalogue(store)
	svc, _ := newImportService(t, store)

	ai := &recordingAI{}
	svc.SetAIMapper(ai)
	svc.SetMatchAdjudicator(ai)
	svc.SetMatchMemory(ai)

	ctx := context.Background()
	session, _, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}

	options := catalog.DefaultImportOptions()
	options.UseAI = false
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: options,
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if ai.touched() != 0 {
		t.Fatalf("AI was used with the switch off: columns=%d values=%d adjudicated=%d memory=%d",
			ai.columns, ai.values, ai.adjudicated, ai.remembered)
	}

	stored, err := store.GetImportSession(ctx, session.PublicID)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if stored.AICalls != 0 {
		t.Errorf("the run recorded %d AI calls with the switch off", stored.AICalls)
	}
}

// The other half of the same property: with the switch on, the ports are
// reachable. A test that only proves "off means off" passes just as well
// against an import that can never call AI at all.
func TestImportReachesTheAIPortsWhenTheSwitchIsOn(t *testing.T) {
	store := newMemoryStore()
	populatedCatalogue(store)
	svc, _ := newImportService(t, store)

	ai := &recordingAI{}
	svc.SetAIMapper(ai)
	svc.SetMatchAdjudicator(ai)
	svc.SetMatchMemory(ai)

	ctx := context.Background()
	session, _, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}

	options := catalog.DefaultImportOptions()
	options.UseAI = true
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: options,
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if ai.touched() == 0 {
		t.Fatal("the switch was on and no AI port was reached")
	}
}

// An import into a catalogue that already holds the products must update them,
// not refuse them and not duplicate them. This is the reported shape of the
// failure: the first file into an empty catalogue works, the second does not.
func TestSecondImportUpdatesRatherThanDuplicating(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	// The catalogue already holds what the file is about to bring, found by the
	// item codes the fixture carries.
	store.existing = map[string]int64{"svc-1": 101, "svc-2": 102, "svc-3": 103}
	store.catalogue = []catalog.MatchProduct{
		{ID: 101, NameAR: "بانادول اكسترا", SKU: "SVC-1"},
		{ID: 102, NameAR: "كتافلام أقراص", SKU: "SVC-2"},
		{ID: 103, NameAR: "اوجمنتين حقن", SKU: "SVC-3"},
	}

	session, _, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}
	prepared, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	})
	if err != nil {
		t.Fatalf("prepare failed against a populated catalogue: %v", err)
	}

	if prepared.Status != catalog.SessionReady {
		t.Fatalf("session is %q, want ready", prepared.Status)
	}
	if prepared.UpdateRows != 3 {
		t.Errorf("staged %d updates, want 3 — an existing product must update", prepared.UpdateRows)
	}
	if prepared.InsertRows != 0 {
		t.Errorf("staged %d inserts, want 0 — an existing product must not duplicate", prepared.InsertRows)
	}
	if prepared.SkipRows != 0 {
		t.Errorf("staged %d skips, want 0 — an existing product must not be blocked", prepared.SkipRows)
	}
}

// The same file into an empty catalogue inserts. Stated alongside the test
// above because the pair is the actual requirement: the importer must do the
// right thing in both states, and it was only ever exercised in one.
func TestFirstImportIntoAnEmptyCatalogueInserts(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}
	prepared, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	})
	if err != nil {
		t.Fatalf("prepare failed against an empty catalogue: %v", err)
	}
	if prepared.InsertRows != 3 {
		t.Errorf("staged %d inserts, want 3", prepared.InsertRows)
	}
}
