package catalog_test

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
)

// Acceptance test for US5 acceptance scenario 4:
// "Given mapping or policy changes and prepare is rerun,
// When new results are saved,
// Then staging reflects the new revision and the catalogue remains unchanged until commit."
func TestRerunSupersedes_IncrementsRevision_OverwritesDecisions_UntouchedCatalogue(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = catalogueOf(catalog.MatchProduct{
		ID: 101, NameAR: "بانادول اكسترا",
	})
	svc, repo := newImportService(t, store)
	ctx := context.Background()

	fixture := "اسم الصنف,كود الصنف,سعر البيع\n" +
		"بانادول اكسترا,PAN-1,55.00\n" +
		"دواء جديد,NEW-1,20.00\n"

	session, _, err := svc.AnalyzeImport(ctx, []byte(fixture), "products.csv", 10)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	// 1. Initial Prepare
	opts1 := catalog.DefaultImportOptions()
	prepSession1, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: opts1,
	})
	if err != nil {
		t.Fatalf("first prepare failed: %v", err)
	}

	if prepSession1.Options.MatchRevision != 1 {
		t.Errorf("first prepare session match_revision = %d, want 1", prepSession1.Options.MatchRevision)
	}
	if len(store.rows) != 2 {
		t.Fatalf("staged %d rows, want 2", len(store.rows))
	}
	for i, r := range store.rows {
		if r.MatchRevision != 1 {
			t.Errorf("row %d match_revision = %d, want 1", i, r.MatchRevision)
		}
		if r.MatchDecision != nil && r.MatchDecision.Revision != 1 {
			t.Errorf("row %d decision revision = %d, want 1", i, r.MatchDecision.Revision)
		}
	}
	if len(store.written) != 0 || len(repo.written) != 0 {
		t.Fatalf("first prepare mutated catalogue: store=%d, repo=%d", len(store.written), len(repo.written))
	}

	// 2. Rerun Prepare with updated settings
	opts2 := catalog.DefaultImportOptions()
	opts2.MinMatchScore = 0.70
	prepSession2, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: opts2,
	})
	if err != nil {
		t.Fatalf("rerun prepare failed: %v", err)
	}

	if prepSession2.Options.MatchRevision != 2 {
		t.Errorf("rerun prepare session match_revision = %d, want 2", prepSession2.Options.MatchRevision)
	}
	if len(store.rows) != 2 {
		t.Fatalf("after rerun staged %d rows, want 2", len(store.rows))
	}
	for i, r := range store.rows {
		if r.MatchRevision != 2 {
			t.Errorf("rerun row %d match_revision = %d, want 2", i, r.MatchRevision)
		}
		if r.MatchDecision != nil && r.MatchDecision.Revision != 2 {
			t.Errorf("rerun row %d decision revision = %d, want 2", i, r.MatchDecision.Revision)
		}
	}
	if len(store.written) != 0 || len(repo.written) != 0 {
		t.Fatalf("rerun mutated catalogue: store=%d, repo=%d", len(store.written), len(repo.written))
	}
}
