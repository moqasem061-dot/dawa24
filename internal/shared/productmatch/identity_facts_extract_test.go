package productmatch

import (
	"testing"
)

func TestExtractRowFacts_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		row       Row
		wantCheck func(t *testing.T, facts IdentityFacts)
	}{
		{
			name: "Arabic-Indic digits: ٥٠٠ مجم",
			row: Row{
				Name: "كونكور ٥٠٠ مجم اقراص",
			},
			wantCheck: func(t *testing.T, facts IdentityFacts) {
				if len(facts.Strengths) == 0 {
					t.Fatalf("expected strength extracted, got none")
				}
				if facts.Strengths[0].Value != "500" || facts.Strengths[0].Unit != "mg" {
					t.Errorf("got %s %s, want 500 mg", facts.Strengths[0].Value, facts.Strengths[0].Unit)
				}
			},
		},
		{
			name: "500 mg vs 0.5 g: 500 mg",
			row: Row{
				Name: "سيتال 500 mg",
			},
			wantCheck: func(t *testing.T, facts IdentityFacts) {
				if len(facts.Strengths) == 0 {
					t.Fatalf("expected strength extracted, got none")
				}
				if facts.Strengths[0].Value != "500" || facts.Strengths[0].Unit != "mg" {
					t.Errorf("got %s %s, want 500 mg", facts.Strengths[0].Value, facts.Strengths[0].Unit)
				}
			},
		},
		{
			name: "500 mg vs 0.5 g: 0.5 g",
			row: Row{
				Name: "سيتال 0.5 g",
			},
			wantCheck: func(t *testing.T, facts IdentityFacts) {
				if len(facts.Strengths) == 0 {
					t.Fatalf("expected strength extracted, got none")
				}
				if facts.Strengths[0].Value != "0.5" || facts.Strengths[0].Unit != "g" {
					t.Errorf("got %s %s, want 0.5 g", facts.Strengths[0].Value, facts.Strengths[0].Unit)
				}
			},
		},
		{
			name: "Concentration ratio: 250 mg/5 mL",
			row: Row{
				Name: "اموكسيل 250 mg/5 ml شراب",
			},
			wantCheck: func(t *testing.T, facts IdentityFacts) {
				if facts.Concentration == nil {
					t.Fatalf("expected concentration ratio, got nil")
				}
				if facts.Concentration.Num.Value != "250" || facts.Concentration.Num.Unit != "mg" {
					t.Errorf("expected Num 250 mg, got %v", facts.Concentration.Num)
				}
				if facts.Concentration.Den.Value != 5 || facts.Concentration.Den.Unit != "ml" {
					t.Errorf("expected Den 5 ml, got %v", facts.Concentration.Den)
				}
			},
		},
		{
			name: "Combination ordered strengths: 5/10 mg",
			row: Row{
				Name: "الكور بلس 5/10 مجم اقراص",
			},
			wantCheck: func(t *testing.T, facts IdentityFacts) {
				if len(facts.Strengths) < 2 {
					t.Fatalf("expected at least 2 strength components, got %d", len(facts.Strengths))
				}
				if facts.Strengths[0].Value != "5" || facts.Strengths[0].Component != 0 {
					t.Errorf("expected first component 5 at pos 0, got %v", facts.Strengths[0])
				}
				if facts.Strengths[1].Value != "10" || facts.Strengths[1].Component != 1 {
					t.Errorf("expected second component 10 at pos 1, got %v", facts.Strengths[1])
				}
			},
		},
		{
			name: "Pack multiplication: 2 × 10",
			row: Row{
				Name: "بنادول 500 مجم 2 × 10 قرص",
			},
			wantCheck: func(t *testing.T, facts IdentityFacts) {
				if facts.Pack == nil {
					t.Fatalf("expected pack facts, got nil")
				}
				if facts.Pack.Count != 20 {
					t.Errorf("expected count 20, got %d", facts.Pack.Count)
				}
				if facts.Pack.SubUnit != "10" {
					t.Errorf("expected sub_unit 10, got %s", facts.Pack.SubUnit)
				}
			},
		},
		{
			name: "Price annotation: س 141 ج",
			row: Row{
				Name: "اتوريزا 10/20 مجم 21 قرص س 141 ج",
			},
			wantCheck: func(t *testing.T, facts IdentityFacts) {
				if len(facts.MoneyEvidence) == 0 {
					t.Errorf("expected money evidence extracted, got none")
				}
				// 141 should NOT be in strengths
				for _, s := range facts.Strengths {
					if s.Value == "141" {
						t.Errorf("141 leaked into strengths: %v", s)
					}
				}
			},
		},
		{
			name: "Huge number overflow protection: 15 digits",
			row: Row{
				Name: "دواء غريب 123456789012345 مجم",
			},
			wantCheck: func(t *testing.T, facts IdentityFacts) {
				if facts.Cert != CertUncertain {
					t.Errorf("expected CertUncertain for huge number, got %v", facts.Cert)
				}
			},
		},
		{
			name: "Nil row does not panic",
			row:  Row{},
			wantCheck: func(t *testing.T, facts IdentityFacts) {
				// No panic
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := ExtractRowFacts(&tt.row, nil)
			tt.wantCheck(t, facts)
		})
	}
}

func TestMasterProductFacts(t *testing.T) {
	p := MasterProduct{
		ID:            101,
		NameAR:        "كونكور 5 مجم 30 قرص",
		DosageForm:    "أقراص",
		Concentration: "5 mg",
		Barcode:       "6221234567890",
		SKU:           "CONC-5",
	}

	facts := p.Facts()
	if len(facts.Strengths) == 0 || facts.Strengths[0].Value != "5" {
		t.Errorf("expected strength 5, got %v", facts.Strengths)
	}
	if len(facts.Identifiers) != 2 {
		t.Errorf("expected 2 identifiers (gtin, catalog_code), got %d", len(facts.Identifiers))
	}
}
