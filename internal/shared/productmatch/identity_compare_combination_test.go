package productmatch

import (
	"testing"
)

func TestCombinationRule(t *testing.T) {
	t.Run("5/10 vs 10/5 is conflict", func(t *testing.T) {
		row := IdentityFacts{
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
				{Value: "10", Unit: "mg", Component: 1},
			},
		}
		prod := IdentityFacts{
			Strengths: []Strength{
				{Value: "10", Unit: "mg", Component: 0},
				{Value: "5", Unit: "mg", Component: 1},
			},
		}

		comp := CompareFacts(row, prod)
		if comp.Attributes["strength"] != OutcomeConflict {
			t.Errorf("expected strength OutcomeConflict, got %v", comp.Attributes["strength"])
		}
		if !hasConflict(comp.Hard, ConflictStrengthComponent) {
			t.Errorf("expected ConflictStrengthComponent in hard conflicts, got %v", comp.Hard)
		}
		if !hasConflict(comp.Hard, ConflictCombination) {
			t.Errorf("expected ConflictCombination in hard conflicts, got %v", comp.Hard)
		}
	})

	t.Run("1 component row vs 2 component product is unstated", func(t *testing.T) {
		row := IdentityFacts{
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
			},
		}
		prod := IdentityFacts{
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
				{Value: "10", Unit: "mg", Component: 1},
			},
		}

		comp := CompareFacts(row, prod)
		if comp.Attributes["strength"] != OutcomeUnstated {
			t.Errorf("expected strength OutcomeUnstated, got %v", comp.Attributes["strength"])
		}
		if len(comp.Hard) != 0 {
			t.Errorf("expected no hard conflicts, got %v", comp.Hard)
		}
	})

	t.Run("2 component row vs 1 component product is conflict", func(t *testing.T) {
		row := IdentityFacts{
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
				{Value: "10", Unit: "mg", Component: 1},
			},
		}
		prod := IdentityFacts{
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
			},
		}

		comp := CompareFacts(row, prod)
		if comp.Attributes["strength"] != OutcomeConflict {
			t.Errorf("expected strength OutcomeConflict, got %v", comp.Attributes["strength"])
		}
		if !hasConflict(comp.Hard, ConflictCombination) {
			t.Errorf("expected ConflictCombination in hard conflicts, got %v", comp.Hard)
		}
	})

	t.Run("2 component matching row and product is agree", func(t *testing.T) {
		row := IdentityFacts{
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
				{Value: "10", Unit: "mg", Component: 1},
			},
		}
		prod := IdentityFacts{
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
				{Value: "10", Unit: "mg", Component: 1},
			},
		}

		comp := CompareFacts(row, prod)
		if comp.Attributes["strength"] != OutcomeAgree {
			t.Errorf("expected strength OutcomeAgree, got %v", comp.Attributes["strength"])
		}
		if len(comp.Hard) != 0 {
			t.Errorf("expected no hard conflicts, got %v", comp.Hard)
		}
	})

	t.Run("unknown component association returns unstated", func(t *testing.T) {
		row := IdentityFacts{
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: -1},
				{Value: "10", Unit: "mg", Component: 1},
			},
		}
		prod := IdentityFacts{
			Strengths: []Strength{
				{Value: "5", Unit: "mg", Component: 0},
				{Value: "10", Unit: "mg", Component: 1},
			},
		}

		comp := CompareFacts(row, prod)
		if comp.Attributes["strength"] != OutcomeUnstated {
			t.Errorf("expected strength OutcomeUnstated when component is unknown, got %v", comp.Attributes["strength"])
		}
	})
}
