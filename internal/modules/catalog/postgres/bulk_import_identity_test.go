package postgres

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// These run without a database on purpose. What they cover is the decision the
// name tier makes once the candidates are in hand, and that decision is where
// the catalogue was being corrupted: the query used to answer it with min(id)
// over a GROUP BY name, which says every row sharing a name is one product.

func fields(nameAR, concentration, form string) productmatch.ProductFields {
	return productmatch.ProductFields{
		NameAR: nameAR, Concentration: concentration, DosageForm: form,
	}
}

func TestChooseByIdentityKeepsStrengthsApart(t *testing.T) {
	// One brand, two strengths, one normalised name. Before this, both rows of
	// an import updated whichever of the two had the lower id.
	shortlist := []nameCandidate{
		{id: 11, fields: fields("اوجمنتين", "375 مجم", "")},
		{id: 12, fields: fields("اوجمنتين", "1 جم", "")},
	}

	got, ok := chooseByIdentity(shortlist, fields("اوجمنتين", "1 جم", ""))
	if !ok {
		t.Fatal("the 1 g row matched nothing; it would have been staged as a new product")
	}
	if got != 12 {
		t.Fatalf("the 1 g row resolved to product %d, want 12", got)
	}

	got, ok = chooseByIdentity(shortlist, fields("اوجمنتين", "375 مجم", ""))
	if !ok || got != 11 {
		t.Fatalf("the 375 mg row resolved to product %d (matched=%v), want 11", got, ok)
	}
}

func TestChooseByIdentityRefusesAStrengthTheCatalogueLacks(t *testing.T) {
	// The right answer here is "no match", which stages the row as the new
	// product it is. Matching it would overwrite a different medicine.
	shortlist := []nameCandidate{
		{id: 11, fields: fields("اوجمنتين", "375 مجم", "")},
	}

	if id, ok := chooseByIdentity(shortlist, fields("اوجمنتين", "1 جم", "")); ok {
		t.Fatalf("a 1 g row matched the 375 mg product (%d)", id)
	}
}

func TestChooseByIdentityMatchesWhenTheCatalogueIsSilent(t *testing.T) {
	// Silence is missing information, not disagreement. A catalogue row that
	// never recorded a strength is still the row an import of that product
	// should update — otherwise every sparse entry gains a duplicate.
	shortlist := []nameCandidate{
		{id: 11, fields: fields("اوجمنتين", "", "")},
	}

	id, ok := chooseByIdentity(shortlist, fields("اوجمنتين", "1 جم", ""))
	if !ok || id != 11 {
		t.Fatalf("resolved to %d (matched=%v), want 11", id, ok)
	}
}

func TestChooseByIdentityPrefersTheRowThatStatesTheSameAttribute(t *testing.T) {
	// Both survive the comparison — the bare row contradicts nothing — but the
	// one that says 1 g is the one meant.
	shortlist := []nameCandidate{
		{id: 11, fields: fields("اوجمنتين", "", "")},
		{id: 12, fields: fields("اوجمنتين", "1 جم", "")},
	}

	id, ok := chooseByIdentity(shortlist, fields("اوجمنتين", "1 جم", ""))
	if !ok || id != 12 {
		t.Fatalf("resolved to %d (matched=%v), want 12", id, ok)
	}
}

func TestChooseByIdentityIsStableAcrossDuplicateCatalogueRows(t *testing.T) {
	// Two catalogue rows nothing distinguishes are a duplicate somebody already
	// created. Picking the same one every time keeps a re-import updating it
	// rather than adding a third.
	shortlist := []nameCandidate{
		{id: 40, fields: fields("اوجمنتين", "1 جم", "")},
		{id: 12, fields: fields("اوجمنتين", "1 جم", "")},
	}

	for i := 0; i < 5; i++ {
		id, ok := chooseByIdentity(shortlist, fields("اوجمنتين", "1 جم", ""))
		if !ok || id != 12 {
			t.Fatalf("resolved to %d (matched=%v), want 12 every time", id, ok)
		}
	}
}

// A row the administrator reviewed as "update product N" commits to product N.
//
// It used to be re-derived here from the name, so a reviewed decision could be
// redirected to a different product — and a row reviewed as an insert could
// arrive as an update to something nobody had chosen. The resolver takes no
// database connection on this path, which is what nil tx checks.
func TestResolveExistingProductsHonoursAReviewedDecision(t *testing.T) {
	prods := []*catalog.Product{
		{ID: 4412, Name: i18n.Text{"ar": "اوجمنتين"}, OrganizationID: 1},
		{ID: 991, Name: i18n.Text{"ar": "بانادول"}, OrganizationID: 1},
	}

	matches, err := resolveExistingProducts(context.Background(), nil, prods)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d decisions, want 2", len(matches))
	}
	if matches[0].id != 4412 || matches[1].id != 991 {
		t.Fatalf("decisions were re-derived: got %d and %d, want 4412 and 991",
			matches[0].id, matches[1].id)
	}
}
