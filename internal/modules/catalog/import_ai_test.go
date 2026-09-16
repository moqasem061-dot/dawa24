package catalog_test

import (
	"context"
	"errors"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// testVocabulary is the closed set a mapping request translates onto.
func testVocabulary() catalog.EnrichVocabulary {
	return catalog.EnrichVocabulary{
		Categories: []catalog.TaxonomyOption{
			{ID: 53, Name: "أدوية"},
			{ID: 57, Name: "عناية بالبشرة الطبية"},
		},
		Brands: []catalog.TaxonomyOption{
			{ID: 900, Name: "جلاكسو"},
		},
		DosageForms: []string{"أقراص", "شراب", "كريم"},
	}
}

var errGatewayDown = errors.New("gateway: unavailable")

// stubMapper is a scripted model, so the import can be driven without a
// Gateway — and so the call counts can be asserted.
type stubMapper struct {
	available   bool
	fail        error
	columnCalls int
	valueCalls  int
	columns     catalog.ColumnMapResult
	values      func(catalog.ValueMapRequest) catalog.ValueMapResult
}

func (s *stubMapper) Available(context.Context) bool { return s.available }

func (s *stubMapper) MapColumns(
	context.Context, catalog.ColumnMapRequest,
) (catalog.ColumnMapResult, error) {
	s.columnCalls++
	if s.fail != nil {
		return catalog.ColumnMapResult{}, s.fail
	}
	return s.columns, nil
}

func (s *stubMapper) MapValues(
	_ context.Context, req catalog.ValueMapRequest,
) (catalog.ValueMapResult, error) {
	s.valueCalls++
	if s.fail != nil {
		return catalog.ValueMapResult{}, s.fail
	}
	if s.values == nil {
		return catalog.ValueMapResult{}, nil
	}
	return s.values(req), nil
}

// A file's distinct values are what gets asked about, not its rows. This is
// what keeps a fifty-thousand-row import to one request per taxonomy.
func TestDistinctValuesFoldsSpellings(t *testing.T) {
	prods := []*catalog.Product{
		{DosageForm: "أقراص"},
		{DosageForm: "اقراص"},   // hamza variant
		{DosageForm: " أقراص "}, // padding
		{DosageForm: "شراب"},
		{DosageForm: ""},
	}

	got := catalog.DistinctValues(prods, func(p *catalog.Product) string { return p.DosageForm })
	if len(got) != 2 {
		t.Fatalf("got %d distinct values (%v), want 2", len(got), got)
	}
}

func TestBuildValueMappingUsesTheCatalogueSpelling(t *testing.T) {
	mapping := catalog.BuildValueMapping(
		[]string{"اقراص", "كبسول"}, []string{"أقراص", "كريم"},
		catalog.ValueMapResult{
			Matches: []catalog.ValueMatch{{Source: "كبسول", Target: "كريم", Confidence: 0.9}},
		})

	// Folding settles the first without asking; the model settles the second.
	if got, ok := mapping.Lookup("اقراص"); !ok || got != "أقراص" {
		t.Errorf("folded lookup = %q,%v, want أقراص", got, ok)
	}
	if got, ok := mapping.Lookup("كبسول"); !ok || got != "كريم" {
		t.Errorf("model lookup = %q,%v, want كريم", got, ok)
	}
}

func TestBuildValueMappingDiscardsLowConfidence(t *testing.T) {
	mapping := catalog.BuildValueMapping(
		[]string{"شيء غامض"}, []string{"أقراص"},
		catalog.ValueMapResult{
			Matches: []catalog.ValueMatch{{Source: "شيء غامض", Target: "أقراص", Confidence: 0.2}},
		})

	if _, ok := mapping.Lookup("شيء غامض"); ok {
		t.Error("a low-confidence translation was accepted")
	}
}

// The column mapper's answer must never overrule a header the deterministic
// mapper matched exactly, and never name a field the importer does not know.
func TestApplyColumnMapGuardsAgainstBadAnswers(t *testing.T) {
	plan := catalog.PlanColumns([]string{"اسم الصنف", "السعر", "عمود غامض"}, nil)

	overrides := catalog.ApplyColumnMap(catalog.ColumnMapResult{
		Columns: []catalog.ColumnAssignment{
			{Column: 2, Field: catalog.FieldNameAR},   // contradicts an exact match
			{Column: 3, Field: "invented_field"},      // not a real field
			{Column: 99, Field: catalog.FieldBarcode}, // outside the sheet
			{Column: 3, Field: catalog.FieldManufacturer},
		},
	}, plan, 3)

	if _, overruled := overrides.Columns[catalog.FieldNameAR]; overruled {
		t.Error("an exact header match was overruled by the model")
	}
	if _, invented := overrides.Columns["invented_field"]; invented {
		t.Error("a field the importer does not know was accepted")
	}
	if _, outside := overrides.Columns[catalog.FieldBarcode]; outside {
		t.Error("a column outside the sheet was accepted")
	}
	if got := overrides.Columns[catalog.FieldManufacturer]; got != 3 {
		t.Errorf("manufacturer column = %d, want 3 — the one good answer must apply", got)
	}
}

// A file with plainly named columns must not spend a request on them.
func TestNeedsColumnHelpOnlyWhenDetectionIsUnsure(t *testing.T) {
	clear := catalog.PlanColumns([]string{"اسم الصنف", "سعر البيع", "الشركة المصنعة"}, nil)
	if catalog.NeedsColumnHelp(clear) {
		t.Error("a plainly named header asked for AI help")
	}

	vague := catalog.PlanColumns([]string{"العمود الأول", "قيمة", "بيان"}, nil)
	if !catalog.NeedsColumnHelp(vague) {
		t.Error("an unrecognisable header did not ask for help")
	}
}

func TestDecodeValueMapToleratesFences(t *testing.T) {
	out, err := catalog.DecodeValueMap(
		"```json\n{\"matches\":[{\"source\":\"اقراص\",\"target\":\"أقراص\",\"confidence\":0.9}]}\n```")
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(out.Matches) != 1 || out.Matches[0].Target != "أقراص" {
		t.Fatalf("decoded %+v", out.Matches)
	}
}
