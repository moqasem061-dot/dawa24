package productmatch_test

import (
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestConvert(t *testing.T) {
	now := time.Now()
	rels := []productmatch.PackRelationship{
		{ID: 1, ProductID: 10, FromUnit: "box", ToUnit: "strip", Factor: 2},
		{ID: 2, ProductID: 10, FromUnit: "strip", ToUnit: "tablet", Factor: 10},
		{ID: 3, ProductID: 10, FromUnit: "pack", ToUnit: "ampoule", Factor: 5, RevokedAt: &now},
	}

	tests := []struct {
		name     string
		qty      int
		from     string
		to       string
		rels     []productmatch.PackRelationship
		wantQty  int
		wantBool bool
	}{
		{
			name:     "identical units",
			qty:      5,
			from:     "tablet",
			to:       "tablet",
			rels:     rels,
			wantQty:  5,
			wantBool: true,
		},
		{
			name:     "direct forward conversion (strip to tablet)",
			qty:      2,
			from:     "strip",
			to:       "tablet",
			rels:     rels,
			wantQty:  20,
			wantBool: true,
		},
		{
			name:     "direct reverse conversion exact integer (tablet to strip)",
			qty:      20,
			from:     "tablet",
			to:       "strip",
			rels:     rels,
			wantQty:  2,
			wantBool: true,
		},
		{
			name:     "direct reverse non-integral division fails",
			qty:      25,
			from:     "tablet",
			to:       "strip",
			rels:     rels,
			wantQty:  0,
			wantBool: false,
		},
		{
			name:     "chained conversion (box to tablet)",
			qty:      3,
			from:     "box",
			to:       "tablet",
			rels:     rels,
			wantQty:  60,
			wantBool: true,
		},
		{
			name:     "revoked relationship ignored",
			qty:      2,
			from:     "pack",
			to:       "ampoule",
			rels:     rels,
			wantQty:  0,
			wantBool: false,
		},
		{
			name:     "no relationship between units",
			qty:      10,
			from:     "box",
			to:       "sachet",
			rels:     rels,
			wantQty:  0,
			wantBool: false,
		},
		{
			name:     "empty rels returns false",
			qty:      10,
			from:     "strip",
			to:       "tablet",
			rels:     nil,
			wantQty:  0,
			wantBool: false,
		},
		{
			name:     "Arabic unit conversion (شريط to قرص)",
			qty:      2,
			from:     "شريط",
			to:       "قرص",
			rels:     rels,
			wantQty:  20,
			wantBool: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotQty, ok := productmatch.Convert(tc.qty, tc.from, tc.to, tc.rels)
			if ok != tc.wantBool {
				t.Fatalf("Convert(%d, %q, %q) bool = %v, want %v", tc.qty, tc.from, tc.to, ok, tc.wantBool)
			}
			if gotQty != tc.wantQty {
				t.Fatalf("Convert(%d, %q, %q) qty = %d, want %d", tc.qty, tc.from, tc.to, gotQty, tc.wantQty)
			}
		})
	}
}

func TestPackOutcome(t *testing.T) {
	rels := []productmatch.PackRelationship{
		{ID: 101, ProductID: 1, FromUnit: "strip", ToUnit: "tablet", Factor: 10},
		{ID: 102, ProductID: 1, FromUnit: "box", ToUnit: "tablet", Factor: 20},
	}

	tests := []struct {
		name       string
		row        productmatch.PackFacts
		prod       productmatch.PackFacts
		rels       []productmatch.PackRelationship
		wantOut    string
		wantRelID  int64
	}{
		{
			name:      "unstated on row",
			row:       productmatch.PackFacts{},
			prod:      productmatch.PackFacts{Count: 20, SubUnit: "tablet"},
			rels:      rels,
			wantOut:   "unstated",
			wantRelID: 0,
		},
		{
			name:      "unstated on prod",
			row:       productmatch.PackFacts{Count: 20, SubUnit: "tablet"},
			prod:      productmatch.PackFacts{},
			rels:      rels,
			wantOut:   "unstated",
			wantRelID: 0,
		},
		{
			name:      "2x10 vs 20 without rel -> needs_relationship, never agree",
			row:       productmatch.PackFacts{Count: 20, Multiplier: 2, SubCount: 10, SubUnit: "strip"},
			prod:      productmatch.PackFacts{Count: 20, SubUnit: "tablet"},
			rels:      nil,
			wantOut:   "needs_relationship",
			wantRelID: 0,
		},
		{
			name:      "2x10 vs 20 with bridging rel -> agree with relID",
			row:       productmatch.PackFacts{Count: 20, Multiplier: 2, SubCount: 10, SubUnit: "strip"},
			prod:      productmatch.PackFacts{Count: 20, SubUnit: "tablet"},
			rels:      rels,
			wantOut:   "agree",
			wantRelID: 101,
		},
		{
			name:      "2x10 vs 2x10 -> agree",
			row:       productmatch.PackFacts{Count: 20, Multiplier: 2, SubCount: 10},
			prod:      productmatch.PackFacts{Count: 20, Multiplier: 2, SubCount: 10},
			rels:      nil,
			wantOut:   "agree",
			wantRelID: 0,
		},
		{
			name:      "2x10 vs 3x10 -> conflict",
			row:       productmatch.PackFacts{Count: 20, Multiplier: 2, SubCount: 10},
			prod:      productmatch.PackFacts{Count: 30, Multiplier: 3, SubCount: 10},
			rels:      nil,
			wantOut:   "conflict",
			wantRelID: 0,
		},
		{
			name:      "2 strips vs 20 tablets without rel -> needs_relationship",
			row:       productmatch.PackFacts{Count: 2, SubUnit: "strip"},
			prod:      productmatch.PackFacts{Count: 20, SubUnit: "tablet"},
			rels:      nil,
			wantOut:   "needs_relationship",
			wantRelID: 0,
		},
		{
			name:      "2 strips vs 20 tablets with rel -> agree",
			row:       productmatch.PackFacts{Count: 2, SubUnit: "strip"},
			prod:      productmatch.PackFacts{Count: 20, SubUnit: "tablet"},
			rels:      rels,
			wantOut:   "agree",
			wantRelID: 101,
		},
		{
			name:      "2 strips vs 30 tablets with rel -> conflict",
			row:       productmatch.PackFacts{Count: 2, SubUnit: "strip"},
			prod:      productmatch.PackFacts{Count: 30, SubUnit: "tablet"},
			rels:      rels,
			wantOut:   "conflict",
			wantRelID: 0,
		},
		{
			name:      "10 tablets vs 20 tablets -> conflict",
			row:       productmatch.PackFacts{Count: 10, SubUnit: "tablet"},
			prod:      productmatch.PackFacts{Count: 20, SubUnit: "tablet"},
			rels:      nil,
			wantOut:   "conflict",
			wantRelID: 0,
		},
		{
			name:      "20 tablets vs 20 tablets -> agree",
			row:       productmatch.PackFacts{Count: 20, SubUnit: "tablet"},
			prod:      productmatch.PackFacts{Count: 20, SubUnit: "tablet"},
			rels:      nil,
			wantOut:   "agree",
			wantRelID: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, relID := productmatch.PackOutcome(tc.row, tc.prod, tc.rels)
			if out != tc.wantOut {
				t.Fatalf("PackOutcome outcome = %q, want %q", out, tc.wantOut)
			}
			if relID != tc.wantRelID {
				t.Fatalf("PackOutcome relID = %d, want %d", relID, tc.wantRelID)
			}
		})
	}
}

func TestGate_Rule8_PackOutcomeIntegration(t *testing.T) {
	prod := &productmatch.MasterProduct{
		ID:            200,
		NameAR:        "كونكور 5 مجم 20 قرص",
		NameEN:        "Concor 5mg 20 tablets",
		DosageForm:    "أقراص",
		Concentration: "5 mg",
	}

	cat := &mockCatalog{
		products: map[int64]*productmatch.MasterProduct{
			200: prod,
		},
	}
	gate := productmatch.NewGate(cat)

	polPackReq := productmatch.DefaultPolicy()
	polPackReq.Required = productmatch.GranularityPack

	// Case 1: Row has 2x10 multi-pack, product has 20 tablets, no rels -> NeedsReview with ReasonPackNeedsRelationship
	rowFactsMulti := productmatch.IdentityFacts{
		Brand:      []productmatch.Token{{Text: "concor", Script: "en"}},
		Strengths:  []productmatch.Strength{{Value: "5", Unit: "mg"}},
		FormFamily: "tablet",
		Pack: &productmatch.PackFacts{
			Count:      20,
			Multiplier: 2,
			SubCount:   10,
			SubUnit:    "strip",
		},
	}

	vMultiNoRel := gate.Evaluate(rowFactsMulti, productmatch.Proposal{
		Path:      productmatch.PathSimilarity,
		ProductID: 200,
		Score:     0.95,
	}, polPackReq)

	if vMultiNoRel.State != matchflow.StateNeedsReview {
		t.Fatalf("expected StateNeedsReview, got %v", vMultiNoRel.State)
	}
	if vMultiNoRel.Pack != matchflow.PackOutcomeNeedsRelationship {
		t.Fatalf("expected PackOutcomeNeedsRelationship, got %v", vMultiNoRel.Pack)
	}
	if !hasReason(vMultiNoRel.Reasons, productmatch.ReasonPackNeedsRelationship) {
		t.Fatalf("expected ReasonPackNeedsRelationship in reasons: %v", vMultiNoRel.Reasons)
	}

	// Case 2: Product has active pack relationship bridging strip to tablet -> Accepted at GranularityPack
	prod.SetPackRelationships([]productmatch.PackRelationship{
		{ID: 888, ProductID: 200, FromUnit: "strip", ToUnit: "tablet", Factor: 10},
	})

	vMultiWithRel := gate.Evaluate(rowFactsMulti, productmatch.Proposal{
		Path:      productmatch.PathSimilarity,
		ProductID: 200,
		Score:     0.95,
	}, polPackReq)

	if vMultiWithRel.State != matchflow.StateAccepted {
		t.Fatalf("expected StateAccepted with rel, got %v", vMultiWithRel.State)
	}
	if vMultiWithRel.Achieved != productmatch.GranularityPack {
		t.Fatalf("expected GranularityPack, got %v", vMultiWithRel.Achieved)
	}
	if vMultiWithRel.Pack != matchflow.PackOutcomeAgree {
		t.Fatalf("expected PackOutcomeAgree, got %v", vMultiWithRel.Pack)
	}
	if vMultiWithRel.RelationshipID == nil || *vMultiWithRel.RelationshipID != 888 {
		t.Fatalf("expected RelationshipID 888, got %v", vMultiWithRel.RelationshipID)
	}
}
