package productmatch

import (
	"testing"
)

func TestHasPlainSiblingAndModifierUnstated(t *testing.T) {
	products := []MasterProduct{
		{
			ID:     1,
			NameAR: "بانادول 500مجم 24 قرص",
			NameEN: "panadol 500 mg 24 tabs",
		},
		{
			ID:     2,
			NameAR: "بانادول اكسترا 500مجم 24 قرص",
			NameEN: "panadol extra 500 mg 24 tabs",
		},
		{
			ID:     3,
			NameAR: "سولو اكسترا 500مجم",
			NameEN: "solo extra 500 mg",
		},
	}

	idx := NewIndex(products)

	pExtra := idx.byID[2]
	if !idx.HasPlainSibling(pExtra) {
		t.Fatalf("expected panadol extra to have plain sibling panadol")
	}

	pSolo := idx.byID[3]
	if idx.HasPlainSibling(pSolo) {
		t.Fatalf("expected solo extra to NOT have a plain sibling")
	}

	t.Run("plain sibling present causes ConflictModifierUnstatedInRow", func(t *testing.T) {
		rowFacts := ExtractRowFacts(&Row{Name: "بانادول 500 مجم"}, nil)
		extraFacts := pExtra.Facts()

		if !extraFacts.HasPlainSibling {
			t.Fatalf("expected extraFacts.HasPlainSibling to be true")
		}

		comp := CompareFacts(rowFacts, extraFacts)
		if !hasConflict(comp.Hard, ConflictModifierUnstatedInRow) {
			t.Errorf("expected ConflictModifierUnstatedInRow in hard conflicts, got %v", comp.Hard)
		}
	})

	t.Run("no plain sibling leaves modifier unstated without hard conflict", func(t *testing.T) {
		rowFacts := ExtractRowFacts(&Row{Name: "سولو 500 مجم"}, nil)
		solofacts := pSolo.Facts()

		if solofacts.HasPlainSibling {
			t.Fatalf("expected solofacts.HasPlainSibling to be false")
		}

		comp := CompareFacts(rowFacts, solofacts)
		if hasConflict(comp.Hard, ConflictModifierUnstatedInRow) {
			t.Errorf("did not expect ConflictModifierUnstatedInRow when no plain sibling exists, got %v", comp.Hard)
		}
		if comp.Attributes["modifier"] != OutcomeUnstated {
			t.Errorf("expected modifier outcome unstated, got %v", comp.Attributes["modifier"])
		}
	})

	t.Run("matching modifier agrees", func(t *testing.T) {
		rowFacts := ExtractRowFacts(&Row{Name: "بانادول اكسترا 500 مجم"}, nil)
		extraFacts := pExtra.Facts()

		comp := CompareFacts(rowFacts, extraFacts)
		if comp.Attributes["modifier"] != OutcomeAgree {
			t.Errorf("expected modifier outcome agree, got %v", comp.Attributes["modifier"])
		}
		if hasConflict(comp.Hard, ConflictModifierUnstatedInRow) {
			t.Errorf("did not expect conflict, got %v", comp.Hard)
		}
	})
}
