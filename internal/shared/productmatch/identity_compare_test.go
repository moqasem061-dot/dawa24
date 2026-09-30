package productmatch

import (
	"testing"
)

func hasConflict(hard []ConflictKind, target ConflictKind) bool {
	for _, c := range hard {
		if c == target {
			return true
		}
	}
	return false
}

func TestCompareFacts_NormativeIdentityExamples(t *testing.T) {
	// Row 1: Brand A 500 mg tablets 20 vs 0.5 g same form/pack -> Agree
	t.Run("Brand_A_500mg_vs_0_5g", func(t *testing.T) {
		row := IdentityFacts{
			Brand:      []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Strengths:  []Strength{{Value: "500", Unit: "mg"}},
			FormFamily: "tablet",
			Pack:       &PackFacts{Count: 20},
		}
		prod := IdentityFacts{
			Brand:      []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Strengths:  []Strength{{Value: "0.5", Unit: "g"}},
			FormFamily: "tablet",
			Pack:       &PackFacts{Count: 20},
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["strength"] != OutcomeAgree {
			t.Errorf("expected strength agree, got %v", comp.Attributes["strength"])
		}
		if len(comp.Hard) > 0 {
			t.Errorf("expected 0 hard conflicts, got %v", comp.Hard)
		}
	})

	// Row 2: Brand A 500 mg vs 250 mg -> Reject strength conflict
	t.Run("Brand_A_500mg_vs_250mg", func(t *testing.T) {
		row := IdentityFacts{
			Brand:     []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Strengths: []Strength{{Value: "500", Unit: "mg"}},
		}
		prod := IdentityFacts{
			Brand:     []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Strengths: []Strength{{Value: "250", Unit: "mg"}},
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["strength"] != OutcomeConflict {
			t.Errorf("expected strength conflict, got %v", comp.Attributes["strength"])
		}
		if !hasConflict(comp.Hard, ConflictStrength) {
			t.Errorf("expected ConflictStrength in hard, got %v", comp.Hard)
		}
	})

	// Row 3: Brand A tablet vs explicit capsule -> Reject form conflict
	t.Run("Brand_A_500mg_tablet_vs_capsule", func(t *testing.T) {
		row := IdentityFacts{
			Brand:      []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Strengths:  []Strength{{Value: "500", Unit: "mg"}},
			FormFamily: "tablet",
		}
		prod := IdentityFacts{
			Brand:      []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Strengths:  []Strength{{Value: "500", Unit: "mg"}},
			FormFamily: "capsule",
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["form"] != OutcomeConflict {
			t.Errorf("expected form conflict, got %v", comp.Attributes["form"])
		}
		if !hasConflict(comp.Hard, ConflictForm) {
			t.Errorf("expected ConflictForm in hard, got %v", comp.Hard)
		}
	})

	// Row 4: Brand A XR vs ordinary release -> Reject release conflict
	t.Run("Brand_A_XR_vs_ordinary", func(t *testing.T) {
		row := IdentityFacts{
			Brand:     []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Modifiers: []string{"xr"},
		}
		prod := IdentityFacts{
			Brand:     []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Modifiers: nil,
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["release"] != OutcomeConflict {
			t.Errorf("expected release conflict, got %v", comp.Attributes["release"])
		}
		if !hasConflict(comp.Hard, ConflictRelease) {
			t.Errorf("expected ConflictRelease in hard, got %v", comp.Hard)
		}
	})

	// Row 5: Brand A Plus 5/10 mg vs 5/20 mg -> Reject component strength conflict
	t.Run("Brand_A_Plus_5_10mg_vs_5_20mg", func(t *testing.T) {
		row := IdentityFacts{
			Brand:     []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Modifiers: []string{"plus"},
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
				{Value: "10", Unit: "mg", Component: 1},
			},
		}
		prod := IdentityFacts{
			Brand:     []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Modifiers: []string{"plus"},
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
				{Value: "20", Unit: "mg", Component: 1},
			},
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["strength"] != OutcomeConflict {
			t.Errorf("expected strength conflict, got %v", comp.Attributes["strength"])
		}
		if !hasConflict(comp.Hard, ConflictStrengthComponent) {
			t.Errorf("expected ConflictStrengthComponent in hard, got %v", comp.Hard)
		}
	})

	// Row 6: Brand A Plus 5 mg vs Two variants differing in omitted component -> Ambiguous/Unstated
	t.Run("Brand_A_Plus_5mg_omitted_component", func(t *testing.T) {
		row := IdentityFacts{
			Brand:     []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Modifiers: []string{"plus"},
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
			},
		}
		prod := IdentityFacts{
			Brand:     []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Modifiers: []string{"plus"},
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
				{Value: "10", Unit: "mg", Component: 1},
			},
		}
		comp := CompareFacts(row, prod)
		// Missing data is not agreement; unstated
		if comp.Attributes["strength"] != OutcomeUnstated {
			t.Errorf("expected strength unstated, got %v", comp.Attributes["strength"])
		}
	})

	// Row 7: Brand A vs Several strengths/packs -> Unstated
	t.Run("Brand_A_unstated_strengths_packs", func(t *testing.T) {
		row := IdentityFacts{
			Brand: []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
		}
		prod := IdentityFacts{
			Brand:     []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Strengths: []Strength{{Value: "500", Unit: "mg"}},
			Pack:      &PackFacts{Count: 20},
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["strength"] != OutcomeUnstated {
			t.Errorf("expected strength unstated, got %v", comp.Attributes["strength"])
		}
		if comp.Attributes["pack"] != OutcomeUnstated {
			t.Errorf("expected pack unstated, got %v", comp.Attributes["pack"])
		}
		if len(comp.Hard) > 0 {
			t.Errorf("expected 0 hard conflicts, got %v", comp.Hard)
		}
	})

	// Row 8: Arabic alias vs English canonical name -> Agree
	t.Run("Arabic_alias_vs_English_name", func(t *testing.T) {
		row := IdentityFacts{
			Brand: []Token{{Text: "كونكور", Script: "ar"}},
		}
		prod := IdentityFacts{
			Brand: []Token{{Text: "concor", Script: "en"}},
		}
		comp := CompareFacts(row, prod)
		// Script alone is not a conflict
		if hasConflict(comp.Hard, ConflictBrand) {
			t.Errorf("unexpected brand conflict for valid transliteration")
		}
	})

	// Row 9: 100 mg/2 mL vs conflicting concentration -> Conflict
	t.Run("Brand_A_100mg_2mL_vs_conflicting_concentration", func(t *testing.T) {
		row := IdentityFacts{
			Brand: []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Concentration: &Ratio{
				Num: Strength{Value: "100", Unit: "mg"},
				Den: Quantity{Value: 2, Unit: "ml"},
			},
		}
		prod := IdentityFacts{
			Brand: []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Concentration: &Ratio{
				Num: Strength{Value: "100", Unit: "mg"},
				Den: Quantity{Value: 5, Unit: "ml"},
			},
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["concentration"] != OutcomeConflict {
			t.Errorf("expected concentration conflict, got %v", comp.Attributes["concentration"])
		}
		if !hasConflict(comp.Hard, ConflictConcentration) {
			t.Errorf("expected ConflictConcentration in hard, got %v", comp.Hard)
		}
	})

	// Row 10: 250 mg/5 mL vs 50 mg/mL equivalent -> Agree
	t.Run("Brand_A_250mg_5mL_vs_50mg_mL_equivalent", func(t *testing.T) {
		row := IdentityFacts{
			Brand: []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Concentration: &Ratio{
				Num: Strength{Value: "250", Unit: "mg"},
				Den: Quantity{Value: 5, Unit: "ml"},
			},
		}
		prod := IdentityFacts{
			Brand: []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Concentration: &Ratio{
				Num: Strength{Value: "50", Unit: "mg"},
				Den: Quantity{Value: 1, Unit: "ml"},
			},
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["concentration"] != OutcomeAgree {
			t.Errorf("expected concentration agree for 250mg/5ml == 50mg/ml, got %v", comp.Attributes["concentration"])
		}
	})

	// Row 11: Brand A 20 tablets vs 10- and 20-tablet packs
	t.Run("Brand_A_20_tablets_pack_comparison", func(t *testing.T) {
		row := IdentityFacts{
			Brand: []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Pack:  &PackFacts{Count: 20},
		}
		prod10 := IdentityFacts{
			Brand: []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Pack:  &PackFacts{Count: 10},
		}
		prod20 := IdentityFacts{
			Brand: []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Pack:  &PackFacts{Count: 20},
		}

		comp10 := CompareFacts(row, prod10)
		if comp10.Attributes["pack"] != OutcomeConflict {
			t.Errorf("expected pack conflict for 20 vs 10, got %v", comp10.Attributes["pack"])
		}

		comp20 := CompareFacts(row, prod20)
		if comp20.Attributes["pack"] != OutcomeAgree {
			t.Errorf("expected pack agree for 20 vs 20, got %v", comp20.Attributes["pack"])
		}
	})

	// Row 12: Brand A 2 strips vs box of 3 strips -> Needs relationship / conflict
	t.Run("Brand_A_2_strips_needs_relationship", func(t *testing.T) {
		row := IdentityFacts{
			Brand: []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Pack:  &PackFacts{Count: 2, SubUnit: "strip"},
		}
		prod := IdentityFacts{
			Brand: []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Pack:  &PackFacts{Count: 30, Container: "box"},
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["pack"] != OutcomeConflict {
			t.Errorf("expected pack conflict without conversion relationship, got %v", comp.Attributes["pack"])
		}
	})

	// Row 13: Product A / Manufacturer X vs Product B shares only Manufacturer X -> Brand conflict
	t.Run("Product_A_vs_Product_B_same_manufacturer_only", func(t *testing.T) {
		row := IdentityFacts{
			Brand:        []Token{{Text: "producta", Script: "en"}},
			Manufacturer: "PharmaCorp",
		}
		prod := IdentityFacts{
			Brand:        []Token{{Text: "productb", Script: "en"}},
			Manufacturer: "PharmaCorp",
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["brand"] != OutcomeConflict {
			t.Errorf("expected brand conflict despite same manufacturer, got %v", comp.Attributes["brand"])
		}
		if !hasConflict(comp.Hard, ConflictBrand) {
			t.Errorf("expected ConflictBrand in hard, got %v", comp.Hard)
		}
	})

	// Row 14: Supplier code 00123 vs unrelated item
	t.Run("Supplier_code_unrelated_item", func(t *testing.T) {
		row := IdentityFacts{
			Brand:       []Token{{Text: "medicinea", Script: "en"}},
			Identifiers: []Identifier{{Namespace: "supplier_code", Value: "00123"}},
		}
		prod := IdentityFacts{
			Brand:       []Token{{Text: "medicineb", Script: "en"}},
			Identifiers: []Identifier{{Namespace: "supplier_code", Value: "00123"}},
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["brand"] != OutcomeConflict {
			t.Errorf("expected brand conflict, got %v", comp.Attributes["brand"])
		}
		if !hasConflict(comp.Hard, ConflictBrand) {
			t.Errorf("expected ConflictBrand in hard, got %v", comp.Hard)
		}
	})

	// Row 15: True target absent from supplied candidates
	t.Run("True_target_absent_candidates", func(t *testing.T) {
		row := IdentityFacts{
			Brand:     []Token{{Text: "uniquebrand", Script: "en"}},
			Strengths: []Strength{{Value: "500", Unit: "mg"}},
		}
		candidate := IdentityFacts{
			Brand:     []Token{{Text: "otherbrand", Script: "en"}},
			Strengths: []Strength{{Value: "250", Unit: "mg"}},
		}
		comp := CompareFacts(row, candidate)
		if !hasConflict(comp.Hard, ConflictBrand) || !hasConflict(comp.Hard, ConflictStrength) {
			t.Errorf("expected brand and strength hard conflicts, got %v", comp.Hard)
		}
	})

	// Row 16: Same raw name, separate structured 10 mg vs 20 mg
	t.Run("Same_name_separate_structured_10mg_vs_20mg", func(t *testing.T) {
		row := IdentityFacts{
			Brand:     []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Strengths: []Strength{{Value: "10", Unit: "mg"}},
		}
		prod := IdentityFacts{
			Brand:     []Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
			Strengths: []Strength{{Value: "20", Unit: "mg"}},
		}
		comp := CompareFacts(row, prod)
		if comp.Attributes["strength"] != OutcomeConflict {
			t.Errorf("expected strength conflict for 10mg vs 20mg, got %v", comp.Attributes["strength"])
		}
		if !hasConflict(comp.Hard, ConflictStrength) {
			t.Errorf("expected ConflictStrength in hard, got %v", comp.Hard)
		}
	})
}
