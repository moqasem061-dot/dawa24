package productmatch_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestReportedImportSuggestions(t *testing.T) {
	cases := []struct{ row, catalog string }{
		{"ابيكساتراك 2.5مجم30ق", "ابيكساتراك 2.5مجم 30 قرص"},
		{"ابيكوبريد 20 مجم بديل س", "ابيكوبريد 20مجم 20 قرص ذائب بالفم"},
		{"ابيكوبريد5 مجم -- بديل س", "ابيكوبريد 5مجم 30 قرص"},
	}
	for _, tc := range cases {
		t.Run(tc.row, func(t *testing.T) {
			idx := productmatch.NewIndex([]productmatch.MasterProduct{{ID: 1, NameAR: tc.catalog}})
			dec := idx.Decide(&productmatch.Row{Name: tc.row}, productmatch.DefaultPolicy())
			if dec.State != matchflow.StateAccepted || dec.ProductID == nil || *dec.ProductID != 1 {
				t.Fatalf("correct suggestion must be accepted: %+v", dec)
			}
		})
	}
}

func TestWeldedShorthandPreservesStrengthConflict(t *testing.T) {
	idx := productmatch.NewIndex([]productmatch.MasterProduct{{ID: 1, NameAR: "ابيكساتراك 5مجم 30 قرص"}})
	dec := idx.Decide(&productmatch.Row{Name: "ابيكساتراك 2.5مجم30ق"}, productmatch.DefaultPolicy())
	if dec.State == matchflow.StateAccepted {
		t.Fatal("another strength must never be accepted")
	}
}

func TestComparedDosageFormsRequireReview(t *testing.T) {
	for _, names := range [][2]string{
		{"Brand D 2% cream against gel", "Brand D 2% cream 30g"},
		{"Brand A 500mg capsule vs tablet", "Brand A 500mg 20 capsules"},
	} {
		index := productmatch.NewIndex([]productmatch.MasterProduct{{ID: 1, NameEN: names[1]}})
		decision := index.Decide(&productmatch.Row{Name: names[0]}, productmatch.DefaultPolicy())
		if decision.State == matchflow.StateAccepted {
			t.Fatalf("contradictory form accepted: %s", names[0])
		}
	}
}
