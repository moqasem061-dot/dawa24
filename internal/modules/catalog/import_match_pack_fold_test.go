package catalog_test

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
)

// The second upload into a populated catalogue used to take the server down.
//
// The first import of a file runs no similarity matching at all — the catalogue
// is empty — so it always succeeded. The second one scores every row, and a
// name carrying a written multiplication ("شاور جل 3×1") panicked in the
// pack-count parser on a matcher worker goroutine, where no recover reaches.
// The process died at the start of the matching phase, and the session was
// later reaped as interrupted with nothing to say why.
func TestSecondImportSurvivesMultipliedPackCounts(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = catalogueOf(
		catalog.MatchProduct{ID: 1, NameAR: "بانادول اكسترا 24 قرص"},
		catalog.MatchProduct{ID: 2, NameAR: "مان زون شاور جل 250 مل"},
	)
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	const fixture = "اسم الصنف,كود الصنف,سعر البيع\n" +
		"مان زون شاور جل 3×1 اينيرجى بووست 250 مل,PK-1,55.00\n" +
		"هير ديلى امبولات علاج تساقط الشعر رجالى 5 × 12 مل,PK-2,42.00\n" +
		"بانادول اكسترا,PK-3,115.00\n"

	session, _, err := svc.AnalyzeImport(ctx, []byte(fixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if len(store.rows) != 3 {
		t.Fatalf("staged %d rows, want 3", len(store.rows))
	}
}
