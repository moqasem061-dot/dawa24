package catalog_test

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// The model must translate onto an existing value, never invent one.
func TestValueMappingRefusesInventedTargets(t *testing.T) {
	sources := []string{"اقراص مغلفه"}
	targets := []string{"أقراص", "شراب"}

	mapping := catalog.BuildValueMapping(sources, targets, catalog.ValueMapResult{
		Matches: []catalog.ValueMatch{{Source: "اقراص مغلفه", Target: "حبوب مغلفة", Confidence: 0.99}},
	})

	if _, ok := mapping.Lookup("اقراص مغلفه"); ok {
		t.Error("a value the catalogue does not have was accepted as a target")
	}
	if len(mapping.Unmatched()) != 1 {
		t.Errorf("unmatched = %v, want the source left for the admin", mapping.Unmatched())
	}
}

// Exact folding settles what it can without asking, and outranks the model.
func TestValueMappingPrefersExactFolding(t *testing.T) {
	mapping := catalog.BuildValueMapping(
		[]string{"اقراص"}, []string{"أقراص"},
		catalog.ValueMapResult{
			Matches: []catalog.ValueMatch{{Source: "اقراص", Target: "شراب", Confidence: 1}},
		})

	got, ok := mapping.Lookup("اقراص")
	if !ok || got != "أقراص" {
		t.Errorf("lookup = %q,%v; folding must win over the model", got, ok)
	}
}

// A row the parser rejected must never be committed, whatever the mode says.
func TestPrepareImportExcludesRejectedRows(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	fixture := "اسم الصنف,كود الصنف,سعر البيع\n" +
		"صنف سليم,SVC-1,55.00\n" +
		"صنف بسعر سالب,SVC-2,-15.00\n"

	session, _, _ := svc.AnalyzeImport(ctx, []byte(fixture), "list.csv", 0)
	prepared, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	})
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if prepared.ParsedRows != 1 {
		t.Fatalf("staged %d rows, want 1 — the negative price must be rejected", prepared.ParsedRows)
	}

	if _, result, err := svc.CommitImport(ctx, session.PublicID); err != nil {
		t.Fatalf("commit failed: %v", err)
	} else if result.Total() != 1 {
		t.Errorf("wrote %d products, want 1", result.Total())
	}
	if len(store.written) != 1 {
		t.Errorf("repository received %d products, want 1", len(store.written))
	}
}

// blockingMapper holds a mapping call until released, so a preparation run can
// be caught mid-flight.
type blockingMapper struct{ release chan struct{} }

func (b *blockingMapper) Available(context.Context) bool { return true }

func (b *blockingMapper) MapColumns(
	context.Context, catalog.ColumnMapRequest,
) (catalog.ColumnMapResult, error) {
	return catalog.ColumnMapResult{}, nil
}

func (b *blockingMapper) MapValues(
	ctx context.Context, _ catalog.ValueMapRequest,
) (catalog.ValueMapResult, error) {
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return catalog.ValueMapResult{}, nil
}

// Preparation runs in the background, so a commit can arrive while the staging
// table is still being filled. Committing then would write a partial catalogue
// built from a partial read.
func TestCommitImportRefusesWhilePreparationIsRunning(t *testing.T) {
	store := newMemoryStore()
	store.vocab = testVocabulary()
	svc, _ := newImportService(t, store)

	blocker := &blockingMapper{release: make(chan struct{})}
	svc.SetAIMapper(blocker)
	ctx := context.Background()

	session, _, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}

	if err := svc.PrepareImportAsync(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd,
		Options: catalog.ImportOptions{
			UseAI: true, AssignCategory: true, AssignDosageForm: true,
		},
	}); err != nil {
		t.Fatalf("prepare could not start: %v", err)
	}

	// Wait until the run is genuinely in flight before trying to commit.
	deadline := time.Now().Add(5 * time.Second)
	for !svc.EnricherRunning(session.PublicID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if _, _, err := svc.CommitImport(ctx, session.PublicID); err == nil {
		t.Error("a session still being prepared was committed")
	}
	if len(store.written) != 0 {
		t.Errorf("a mid-preparation commit wrote %d products", len(store.written))
	}

	close(blocker.release)

	// Once it finishes, the same session commits normally.
	for i := 0; i < 200; i++ {
		if progress, ok := svc.ImportProgress(session.PublicID); ok && progress.Phase.Terminal() {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if _, _, err := svc.CommitImport(ctx, session.PublicID); err != nil {
		t.Fatalf("commit after preparation finished: %v", err)
	}
	if len(store.written) == 0 {
		t.Error("nothing was written after a successful preparation")
	}
}
