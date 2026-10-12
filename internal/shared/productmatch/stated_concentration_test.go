package productmatch

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestProductionConcentrationComparison(t *testing.T) {
	for _, tc := range []struct {
		row, catalog string
		accept       bool
	}{
		{"Concor 100mg/2mL injection", "Concor 100mg/5mL injection", false},
		{"Concor 10mg/5mL vial", "Concor 50mg/25mL vial", false},
		{"Concor 250mg/5mL syrup", "Concor 50mg/mL syrup", true},
		{"Concor 10/20mg tablets", "Concor 20/10mg tablets", true},
	} {
		t.Run(tc.row, func(t *testing.T) {
			index := NewIndex([]MasterProduct{{ID: 1, NameEN: tc.catalog}})
			dec := index.Decide(&Row{Name: tc.row}, DefaultPolicy())
			if (dec.State == matchflow.StateAccepted) != tc.accept {
				t.Fatalf("decision: %+v", dec)
			}
			if !tc.accept && len(index.HardConflicts(&Row{Name: tc.row}, 1)) == 0 {
				t.Fatal("concentration conflict bypassed by non-scoring paths")
			}
		})
	}
}

func TestConcentrationRequiresWholeVolumeUnit(t *testing.T) {
	if statedConcentration("Concor 10mg/2ملجم") != nil {
		t.Fatal("milligrams parsed as millilitres")
	}
	if statedConcentration("Concor 10mg/2مللي") == nil {
		t.Fatal("full Arabic millilitre unit omitted")
	}
}
