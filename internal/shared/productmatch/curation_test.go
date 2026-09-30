package productmatch

import (
	"encoding/json"
	"testing"
)

func TestApplyCuration_Overrides(t *testing.T) {
	facts := IdentityFacts{
		Brand:      []Token{{Text: "Panadol"}},
		Strengths:  []Strength{{Value: "500", Unit: "mg", Prov: ProvOriginal}},
		FormFamily: "tablet",
	}

	curations := []CuratedFact{
		{
			Attribute: "strength",
			Value:     json.RawMessage(`"1000 mg"`),
		},
		{
			Attribute: "form",
			Value:     json.RawMessage(`"capsule"`),
		},
		{
			Attribute: "brand",
			Value:     json.RawMessage(`"Panadol Extra"`),
		},
		{
			Attribute: "pack",
			Value:     json.RawMessage(`24`),
		},
	}

	ApplyCuration(&facts, curations, nil)

	if len(facts.Strengths) != 1 || facts.Strengths[0].Value != "1000" || facts.Strengths[0].Prov != ProvCorrected {
		t.Errorf("expected strength 1000 with ProvCorrected, got %+v", facts.Strengths)
	}

	if facts.FormFamily != "capsule" || facts.Prov != ProvCorrected {
		t.Errorf("expected capsule form with ProvCorrected, got %s prov %s", facts.FormFamily, facts.Prov)
	}

	if len(facts.Brand) != 1 || facts.Brand[0].Text != "Panadol Extra" {
		t.Errorf("expected Panadol Extra brand, got %+v", facts.Brand)
	}

	if facts.Pack == nil || facts.Pack.Count != 24 || facts.Pack.Prov != ProvCorrected {
		t.Errorf("expected pack 24 with ProvCorrected, got %+v", facts.Pack)
	}
}

func TestNewIndexWithCuration(t *testing.T) {
	prods := []MasterProduct{
		{ID: 1, NameAR: "كونكور 5 مجم 30 قرص", NameEN: "Concor 5mg 30 tab", DosageForm: "أقراص", Concentration: "5 mg", Unit: "30"},
		{ID: 2, NameAR: "بانادول 500 مجم", NameEN: "Panadol 500mg", DosageForm: "أقراص", Concentration: "500 mg", Unit: "24"},
	}

	curatedFacts := map[int64][]CuratedFact{
		2: {
			{
				ProductID: 2,
				Attribute: "strength",
				Value:     json.RawMessage(`"1000 mg"`),
			},
		},
	}

	curatedRels := map[int64][]PackRelationship{
		1: {
			{
				ProductID: 1,
				FromUnit:  "علبة",
				ToUnit:    "قرص",
				Factor:    30,
			},
		},
	}

	idx := NewIndexWithCuration(prods, curatedFacts, curatedRels)
	if idx == nil {
		t.Fatal("expected non-nil index")
	}

	// Product 1 facts unchanged for strength
	p1, ok := idx.Lookup(1)
	if !ok {
		t.Fatal("expected product 1 in index")
	}
	f1 := p1.Facts()
	if len(f1.Strengths) == 0 || f1.Strengths[0].Value != "5" {
		t.Errorf("expected product 1 strength 5, got %+v", f1.Strengths)
	}

	// Product 2 facts updated by curation
	p2, ok := idx.Lookup(2)
	if !ok {
		t.Fatal("expected product 2 in index")
	}
	f2 := p2.Facts()
	if len(f2.Strengths) == 0 || f2.Strengths[0].Value != "1000" || f2.Strengths[0].Prov != ProvCorrected {
		t.Errorf("expected product 2 curated strength 1000 with ProvCorrected, got %+v", f2.Strengths)
	}
}
