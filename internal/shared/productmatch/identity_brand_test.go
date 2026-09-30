package productmatch

import (
	"testing"
)

func TestBrandConflictAndAlternativePhrases(t *testing.T) {
	t.Run("StripAlternativeClause", func(t *testing.T) {
		cases := []struct {
			in   string
			want string
		}{
			{"Product A, alternative to Product B", "Product A"},
			{"Brand A 0.5g, alternative to Brand B", "Brand A 0.5g"},
			{"Brand A alternative to Brand B", "Brand A"},
			{"Brand A substitute for Brand B", "Brand A"},
			{"براند أ بديل براند ب", "براند أ"},
			{"براند أ بديل لـ براند ب", "براند أ"},
			{"Product A (بديل Product B)", "Product A"},
			{"Product A (alternative to Product B)", "Product A"},
			{"Panadol 500mg", "Panadol 500mg"},
		}

		for _, tc := range cases {
			got := StripAlternativeClause(tc.in)
			if got != tc.want {
				t.Errorf("StripAlternativeClause(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	})

	t.Run("Product A alternative to Product B against Product A agrees", func(t *testing.T) {
		row := &Row{Name: "Product A, alternative to Product B"}
		rowFacts := ExtractRowFacts(row, nil)

		candA := &MasterProduct{ID: 1, NameEN: "Product A 500mg"}
		candFactsA := candA.Facts()

		comp := CompareFacts(rowFacts, candFactsA)
		if comp.Attributes["brand"] != OutcomeAgree {
			t.Errorf("expected brand OutcomeAgree for Product A, got %v", comp.Attributes["brand"])
		}
		if hasConflict(comp.Hard, ConflictBrand) {
			t.Errorf("did not expect ConflictBrand for Product A, got %v", comp.Hard)
		}
	})

	t.Run("Product A alternative to Product B against Product B conflicts", func(t *testing.T) {
		row := &Row{Name: "Product A, alternative to Product B"}
		rowFacts := ExtractRowFacts(row, nil)

		candB := &MasterProduct{ID: 2, NameEN: "Product B 500mg"}
		candFactsB := candB.Facts()

		comp := CompareFacts(rowFacts, candFactsB)
		if comp.Attributes["brand"] != OutcomeConflict {
			t.Errorf("expected brand OutcomeConflict for Product B, got %v", comp.Attributes["brand"])
		}
		if !hasConflict(comp.Hard, ConflictBrand) {
			t.Errorf("expected ConflictBrand in hard conflicts for Product B, got %v", comp.Hard)
		}
	})

	t.Run("Arabic alternative against alternative product conflicts", func(t *testing.T) {
		row := &Row{Name: "براند أ بديل براند ب"}
		rowFacts := ExtractRowFacts(row, nil)

		candB := &MasterProduct{ID: 2, NameAR: "براند ب 500مجم"}
		candFactsB := candB.Facts()

		comp := CompareFacts(rowFacts, candFactsB)
		if comp.Attributes["brand"] != OutcomeConflict {
			t.Errorf("expected brand OutcomeConflict for Arabic candidate B, got %v", comp.Attributes["brand"])
		}
		if !hasConflict(comp.Hard, ConflictBrand) {
			t.Errorf("expected ConflictBrand in hard conflicts for Arabic candidate B, got %v", comp.Hard)
		}
	})

	t.Run("Direct facts with distinct identifiable brand tokens conflict", func(t *testing.T) {
		rowFacts := IdentityFacts{
			Brand: []Token{{Text: "proda", Script: "en"}},
		}
		candFacts := IdentityFacts{
			Brand: []Token{{Text: "prodb", Script: "en"}},
		}

		comp := CompareFacts(rowFacts, candFacts)
		if comp.Attributes["brand"] != OutcomeConflict {
			t.Errorf("expected brand OutcomeConflict, got %v", comp.Attributes["brand"])
		}
		if !hasConflict(comp.Hard, ConflictBrand) {
			t.Errorf("expected ConflictBrand in hard conflicts, got %v", comp.Hard)
		}
	})
}
