package productmatch_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestNormalizeConcentration(t *testing.T) {
	tests := []struct {
		name       string
		ratio      *productmatch.Ratio
		wantUnit   string
		wantNumRat string // e.g. "50/1"
		wantValid  bool
	}{
		{
			name: "250 mg / 5 mL -> 50/1 mg/ml",
			ratio: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "250", Unit: "mg"},
				Den: productmatch.Quantity{Value: 5, Unit: "ml"},
			},
			wantUnit:   "mg/ml",
			wantNumRat: "50/1",
			wantValid:  true,
		},
		{
			name: "50 mg / 1 mL -> 50/1 mg/ml",
			ratio: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "50", Unit: "mg"},
				Den: productmatch.Quantity{Value: 1, Unit: "ml"},
			},
			wantUnit:   "mg/ml",
			wantNumRat: "50/1",
			wantValid:  true,
		},
		{
			name: "0.5 g / 10 mL -> 50/1 mg/ml (g to mg)",
			ratio: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "0.5", Unit: "g"},
				Den: productmatch.Quantity{Value: 10, Unit: "ml"},
			},
			wantUnit:   "mg/ml",
			wantNumRat: "50/1",
			wantValid:  true,
		},
		{
			name: "100 mg / 0.1 L -> 1/1 mg/ml (L to mL)",
			ratio: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "100", Unit: "mg"},
				Den: productmatch.Quantity{Value: 0.1, Unit: "l"},
			},
			wantUnit:   "mg/ml",
			wantNumRat: "1/1",
			wantValid:  true,
		},
		{
			name: "2% cream -> 2/1 % (percent never converted)",
			ratio: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "2", Unit: "%"},
			},
			wantUnit:   "%",
			wantNumRat: "2/1",
			wantValid:  true,
		},
		{
			name: "1000 IU / mL -> 1000/1 iu/ml (IU never converted)",
			ratio: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "1000", Unit: "iu"},
				Den: productmatch.Quantity{Value: 1, Unit: "ml"},
			},
			wantUnit:   "iu/ml",
			wantNumRat: "1000/1",
			wantValid:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := productmatch.NormalizeConcentration(tc.ratio)
			if got.Valid != tc.wantValid {
				t.Fatalf("NormalizeConcentration valid = %v, want %v", got.Valid, tc.wantValid)
			}
			if got.Unit != tc.wantUnit {
				t.Errorf("NormalizeConcentration unit = %q, want %q", got.Unit, tc.wantUnit)
			}
			if got.Num.String() != tc.wantNumRat {
				t.Errorf("NormalizeConcentration num = %q, want %q", got.Num.String(), tc.wantNumRat)
			}
		})
	}
}

func TestCompareConcentrations(t *testing.T) {
	tests := []struct {
		name     string
		rowConc  *productmatch.Ratio
		prodConc *productmatch.Ratio
		want     productmatch.Outcome
	}{
		{
			name: "Normative: 250 mg/5 mL == 50 mg/mL",
			rowConc: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "250", Unit: "mg"},
				Den: productmatch.Quantity{Value: 5, Unit: "ml"},
			},
			prodConc: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "50", Unit: "mg"},
				Den: productmatch.Quantity{Value: 1, Unit: "ml"},
			},
			want: productmatch.OutcomeAgree,
		},
		{
			name: "Normative: 100 mg/2 mL vs 100 mg/5 mL (50 mg/ml vs 20 mg/ml) -> Conflict",
			rowConc: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "100", Unit: "mg"},
				Den: productmatch.Quantity{Value: 2, Unit: "ml"},
			},
			prodConc: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "100", Unit: "mg"},
				Den: productmatch.Quantity{Value: 5, Unit: "ml"},
			},
			want: productmatch.OutcomeConflict,
		},
		{
			name: "0.25 g / 5 mL == 50 mg/mL",
			rowConc: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "0.25", Unit: "g"},
				Den: productmatch.Quantity{Value: 5, Unit: "ml"},
			},
			prodConc: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "50", Unit: "mg"},
				Den: productmatch.Quantity{Value: 1, Unit: "ml"},
			},
			want: productmatch.OutcomeAgree,
		},
		{
			name: "2% vs 20 mg/mL -> Conflict (percent never converted)",
			rowConc: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "2", Unit: "%"},
			},
			prodConc: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "20", Unit: "mg"},
				Den: productmatch.Quantity{Value: 1, Unit: "ml"},
			},
			want: productmatch.OutcomeConflict,
		},
		{
			name: "1000 IU vs 1000 mg -> Conflict (IU never converted)",
			rowConc: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "1000", Unit: "iu"},
			},
			prodConc: &productmatch.Ratio{
				Num: productmatch.Strength{Value: "1000", Unit: "mg"},
			},
			want: productmatch.OutcomeConflict,
		},
		{
			name:     "one side unstated -> OutcomeUnstated",
			rowConc:  nil,
			prodConc: &productmatch.Ratio{Num: productmatch.Strength{Value: "50", Unit: "mg"}},
			want:     productmatch.OutcomeUnstated,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := productmatch.CompareConcentrations(tc.rowConc, tc.prodConc)
			if got != tc.want {
				t.Fatalf("CompareConcentrations = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCompareFacts_NormativeConcentrationEquivalence(t *testing.T) {
	// Case 1: Brand A 250 mg/5 mL 100 mL syrup vs Brand A 50 mg/mL 100 mL syrup
	row := productmatch.IdentityFacts{
		Brand:      []productmatch.Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
		FormFamily: "syrup",
		Concentration: &productmatch.Ratio{
			Num: productmatch.Strength{Value: "250", Unit: "mg"},
			Den: productmatch.Quantity{Value: 5, Unit: "ml"},
		},
		TotalContent: &productmatch.Quantity{Value: 100, Unit: "ml"},
	}
	prod := productmatch.IdentityFacts{
		Brand:      []productmatch.Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
		FormFamily: "syrup",
		Concentration: &productmatch.Ratio{
			Num: productmatch.Strength{Value: "50", Unit: "mg"},
			Den: productmatch.Quantity{Value: 1, Unit: "ml"},
		},
		TotalContent: &productmatch.Quantity{Value: 100, Unit: "ml"},
	}

	comp := productmatch.CompareFacts(row, prod)
	if comp.Attributes["concentration"] != productmatch.OutcomeAgree {
		t.Errorf("expected concentration OutcomeAgree, got %v", comp.Attributes["concentration"])
	}
	if comp.Attributes["strength"] != productmatch.OutcomeAgree {
		t.Errorf("expected strength OutcomeAgree via concentration equivalence, got %v", comp.Attributes["strength"])
	}
	if len(comp.Hard) > 0 {
		t.Errorf("expected zero hard conflicts for equivalent concentrations, got %v", comp.Hard)
	}

	// Case 2: Brand A 100 mg/2 mL vs Brand A 100 mg/5 mL -> Conflict
	prodConf := productmatch.IdentityFacts{
		Brand:      []productmatch.Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
		FormFamily: "syrup",
		Concentration: &productmatch.Ratio{
			Num: productmatch.Strength{Value: "100", Unit: "mg"},
			Den: productmatch.Quantity{Value: 5, Unit: "ml"},
		},
	}
	rowInj := productmatch.IdentityFacts{
		Brand:      []productmatch.Token{{Text: "brand", Script: "en"}, {Text: "a", Script: "en"}},
		FormFamily: "syrup",
		Concentration: &productmatch.Ratio{
			Num: productmatch.Strength{Value: "100", Unit: "mg"},
			Den: productmatch.Quantity{Value: 2, Unit: "ml"},
		},
	}
	compConf := productmatch.CompareFacts(rowInj, prodConf)
	if compConf.Attributes["concentration"] != productmatch.OutcomeConflict {
		t.Errorf("expected concentration OutcomeConflict, got %v", compConf.Attributes["concentration"])
	}
	hasConcConflict := false
	for _, h := range compConf.Hard {
		if h == productmatch.ConflictConcentration {
			hasConcConflict = true
		}
	}
	if !hasConcConflict {
		t.Errorf("expected ConflictConcentration in hard conflicts, got %v", compConf.Hard)
	}
}
