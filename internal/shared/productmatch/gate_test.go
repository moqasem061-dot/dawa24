package productmatch_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockCatalog struct {
	products map[int64]*productmatch.MasterProduct
}

func (m *mockCatalog) Lookup(id int64) (*productmatch.MasterProduct, bool) {
	p, ok := m.products[id]
	return p, ok
}

func hasReason(reasons []productmatch.Reason, code productmatch.ReasonCode) bool {
	for _, r := range reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}

func TestGate_Rule1_ExistenceAndEligibility(t *testing.T) {
	cat := &mockCatalog{products: make(map[int64]*productmatch.MasterProduct)}
	gate := productmatch.NewGate(cat)
	pol := productmatch.DefaultPolicy()

	v := gate.Evaluate(productmatch.IdentityFacts{}, productmatch.Proposal{
		Path:      productmatch.PathSimilarity,
		ProductID: 99999, // nonexistent
	}, pol)

	if v.Eligible {
		t.Errorf("expected not eligible")
	}
	if v.State != matchflow.StateNoCandidate {
		t.Errorf("expected StateNoCandidate, got %v", v.State)
	}
	if !hasReason(v.Reasons, productmatch.ReasonNoCandidate) {
		t.Errorf("expected ReasonNoCandidate")
	}
}

func TestGate_Rule2_HardConflicts_IdentifierAndReviewer(t *testing.T) {
	cat := &mockCatalog{
		products: map[int64]*productmatch.MasterProduct{
			100: {
				ID:            100,
				NameAR:        "كونكور 5 مجم",
				Concentration: "5 mg",
			},
		},
	}
	gate := productmatch.NewGate(cat)
	pol := productmatch.DefaultPolicy()

	// Row has 10 mg vs Product 5 mg -> Strength hard conflict
	rowFacts := productmatch.IdentityFacts{
		Brand:     []productmatch.Token{{Text: "concor", Script: "en"}},
		Strengths: []productmatch.Strength{{Value: "10", Unit: "mg"}},
	}

	// 1. Identifier path with strength conflict ⇒ not eligible, identifier_vs_description
	t.Run("identifier_strength_conflict", func(t *testing.T) {
		v := gate.Evaluate(rowFacts, productmatch.Proposal{
			Path:             productmatch.PathIdentifier,
			ProductID:        100,
			IdentifierValid:  true,
			IdentifierUnique: true,
		}, pol)
		if v.Eligible {
			t.Errorf("expected not eligible for identifier proposal with strength conflict")
		}
		if v.State != matchflow.StateNeedsReview {
			t.Errorf("expected StateNeedsReview, got %v", v.State)
		}
		if !hasReason(v.Reasons, productmatch.ReasonStrengthConflict) {
			t.Errorf("expected ReasonStrengthConflict")
		}
		if !hasReason(v.Reasons, productmatch.ReasonIdentifierVsDescription) {
			t.Errorf("expected ReasonIdentifierVsDescription")
		}
	})

	// 2. Reviewer path with hard conflict without correction ⇒ not eligible
	t.Run("reviewer_hard_conflict_without_correction", func(t *testing.T) {
		v := gate.Evaluate(rowFacts, productmatch.Proposal{
			Path:      productmatch.PathReviewer,
			ProductID: 100,
		}, pol)

		if v.Eligible {
			t.Errorf("expected not eligible for reviewer proposal with uncorrected hard conflict")
		}
		if v.State != matchflow.StateNeedsReview {
			t.Errorf("expected StateNeedsReview, got %v", v.State)
		}
		if !hasReason(v.Reasons, productmatch.ReasonStrengthConflict) {
			t.Errorf("expected ReasonStrengthConflict")
		}
	})
}

func TestGate_Rule3_IdentifierPathExtras(t *testing.T) {
	cat := &mockCatalog{
		products: map[int64]*productmatch.MasterProduct{
			100: {ID: 100, NameAR: "كونكور 5 مجم", Concentration: "5 mg"},
		},
	}
	gate := productmatch.NewGate(cat)
	pol := productmatch.DefaultPolicy()

	rowFacts := productmatch.IdentityFacts{
		Brand:     []productmatch.Token{{Text: "concor", Script: "en"}},
		Strengths: []productmatch.Strength{{Value: "5", Unit: "mg"}},
	}

	// Invalid check digit / checksum
	vInvalid := gate.Evaluate(rowFacts, productmatch.Proposal{
		Path:            productmatch.PathIdentifier,
		ProductID:       100,
		IdentifierValid: false,
	}, pol)
	if vInvalid.Eligible || !hasReason(vInvalid.Reasons, productmatch.ReasonIdentifierInvalid) {
		t.Errorf("expected invalid identifier handling, got %+v", vInvalid)
	}

	// Duplicate identifier
	vDup := gate.Evaluate(rowFacts, productmatch.Proposal{
		Path:             productmatch.PathIdentifier,
		ProductID:        100,
		IdentifierValid:  true,
		IdentifierUnique: false,
	}, pol)
	if vDup.Eligible || !hasReason(vDup.Reasons, productmatch.ReasonIdentifierDuplicate) {
		t.Errorf("expected duplicate identifier handling, got %+v", vDup)
	}
}

func TestGate_Rule4_Uniqueness_AmbiguousSiblings(t *testing.T) {
	cat := &mockCatalog{
		products: map[int64]*productmatch.MasterProduct{
			100: {ID: 100, NameAR: "كونكور 5 مجم", Concentration: "5 mg"},
			200: {ID: 200, NameAR: "كونكور 5 مجم", Concentration: "5 mg"},
		},
	}
	gate := productmatch.NewGate(cat)
	pol := productmatch.DefaultPolicy()

	rowFacts := productmatch.IdentityFacts{
		Brand:     []productmatch.Token{{Text: "concor", Script: "en"}},
		Strengths: []productmatch.Strength{{Value: "5", Unit: "mg"}},
	}

	v := gate.Evaluate(rowFacts, productmatch.Proposal{
		Path:      productmatch.PathSimilarity,
		ProductID: 100,
		Score:     0.95,
		Siblings:  []int64{100, 200},
	}, pol)

	if !v.Eligible {
		t.Errorf("expected eligible candidates")
	}
	if v.State != matchflow.StateAmbiguous {
		t.Errorf("expected StateAmbiguous, got %v", v.State)
	}
	if !hasReason(v.Reasons, productmatch.ReasonAmbiguousSiblings) {
		t.Errorf("expected ReasonAmbiguousSiblings")
	}
}

func TestGate_Rule5_OmittedAttributes(t *testing.T) {
	cat := &mockCatalog{
		products: map[int64]*productmatch.MasterProduct{
			100: {ID: 100, NameAR: "كونكور 5 مجم", Concentration: "5 mg"},
		},
	}
	gate := productmatch.NewGate(cat)

	// Row omits strength
	rowFacts := productmatch.IdentityFacts{
		Brand: []productmatch.Token{{Text: "concor", Script: "en"}},
	}

	polDisallow := productmatch.DefaultPolicy()
	polDisallow.AllowOmittedAttrs = false

	v := gate.Evaluate(rowFacts, productmatch.Proposal{
		Path:      productmatch.PathSimilarity,
		ProductID: 100,
		Score:     0.95,
	}, polDisallow)

	if v.State != matchflow.StateNeedsReview {
		t.Errorf("expected StateNeedsReview when omitted attrs disallowed, got %v", v.State)
	}
	if !hasReason(v.Reasons, productmatch.ReasonAttributeUnstated) {
		t.Errorf("expected ReasonAttributeUnstated")
	}
}

func TestGate_Rule6_BlindSimilarity(t *testing.T) {
	cat := &mockCatalog{
		products: map[int64]*productmatch.MasterProduct{
			100: {ID: 100, NameAR: "كونكور 5 مجم", Concentration: "5 mg"},
		},
	}
	gate := productmatch.NewGate(cat)
	pol := productmatch.DefaultPolicy()

	// Row without agreed dose, letters only, score 0.85 < 0.90
	rowFacts := productmatch.IdentityFacts{
		Brand: []productmatch.Token{{Text: "concor", Script: "en"}},
	}

	v := gate.Evaluate(rowFacts, productmatch.Proposal{
		Path:        productmatch.PathSimilarity,
		ProductID:   100,
		Score:       0.85,
		LettersOnly: true,
	}, pol)

	if v.State != matchflow.StateNeedsReview {
		t.Errorf("expected StateNeedsReview for letters-only blind match, got %v", v.State)
	}
	if !hasReason(v.Reasons, productmatch.ReasonSimilarityLettersOnly) {
		t.Errorf("expected ReasonSimilarityLettersOnly")
	}
}

func TestGate_Rule7_ThresholdsAndQualifications(t *testing.T) {
	cat := &mockCatalog{
		products: map[int64]*productmatch.MasterProduct{
			100: {ID: 100, NameAR: "كونكور 5 مجم", Concentration: "5 mg"},
		},
	}
	gate := productmatch.NewGate(cat)
	pol := productmatch.DefaultPolicy()

	rowFacts := productmatch.IdentityFacts{
		Brand:     []productmatch.Token{{Text: "concor", Script: "en"}},
		Strengths: []productmatch.Strength{{Value: "5", Unit: "mg"}},
	}

	// Score < MinReview (0.25) -> StateNoCandidate
	vBelowMinReview := gate.Evaluate(rowFacts, productmatch.Proposal{
		Path:      productmatch.PathSimilarity,
		ProductID: 100,
		Score:     0.20,
	}, pol)
	if vBelowMinReview.State != matchflow.StateNoCandidate || !hasReason(vBelowMinReview.Reasons, productmatch.ReasonNoCandidate) {
		t.Errorf("expected StateNoCandidate for score < MinReview, got %+v", vBelowMinReview)
	}

	// Score < MinStrong (0.60)
	vLowScore := gate.Evaluate(rowFacts, productmatch.Proposal{
		Path:      productmatch.PathSimilarity,
		ProductID: 100,
		Score:     0.55,
	}, pol)
	if vLowScore.State != matchflow.StateNeedsReview {
		t.Errorf("expected StateNeedsReview for low score, got %v", vLowScore.State)
	}

	// AI unqualified
	vAIUnqualified := gate.Evaluate(rowFacts, productmatch.Proposal{
		Path:      productmatch.PathAI,
		ProductID: 100,
		Evidence:  &productmatch.AIEvidence{Qualified: false},
	}, pol)
	if vAIUnqualified.State != matchflow.StateNeedsReview || !hasReason(vAIUnqualified.Reasons, productmatch.ReasonAIRejectedUnqualified) {
		t.Errorf("expected AI unqualified rejection, got %+v", vAIUnqualified)
	}

	// AI disagrees
	vAIDisagrees := gate.Evaluate(rowFacts, productmatch.Proposal{
		Path:      productmatch.PathAI,
		ProductID: 100,
		Evidence:  &productmatch.AIEvidence{Qualified: true, Disagrees: true},
	}, pol)
	if vAIDisagrees.State != matchflow.StateNeedsReview || !hasReason(vAIDisagrees.Reasons, productmatch.ReasonAIDisagrees) {
		t.Errorf("expected AI disagrees rejection, got %+v", vAIDisagrees)
	}

	// Knowledge stale
	vStale := gate.Evaluate(rowFacts, productmatch.Proposal{
		Path:           productmatch.PathKnowledge,
		ProductID:      100,
		KnowledgeStale: true,
	}, pol)
	if vStale.State != matchflow.StateNeedsReview || !hasReason(vStale.Reasons, productmatch.ReasonKnowledgeStale) {
		t.Errorf("expected knowledge stale rejection, got %+v", vStale)
	}
}

func TestGate_Rule8_GranularityAndReviewerConfirmed(t *testing.T) {
	cat := &mockCatalog{
		products: map[int64]*productmatch.MasterProduct{
			100: {
				ID:            100,
				NameAR:        "كونكور 5 مجم أقراص 30",
				Concentration: "5 mg",
				DosageForm:    "tablets",
			},
		},
	}
	gate := productmatch.NewGate(cat)

	// Row without pack
	rowFactsNoPack := productmatch.IdentityFacts{
		Brand:      []productmatch.Token{{Text: "concor", Script: "en"}},
		Strengths:  []productmatch.Strength{{Value: "5", Unit: "mg"}},
		FormFamily: "tablet",
	}

	polPackReq := productmatch.DefaultPolicy()
	polPackReq.Required = productmatch.GranularityPack

	// Required pack, but row lacks pack -> Achieved product < Required pack -> needs_review
	vPackUnresolved := gate.Evaluate(rowFactsNoPack, productmatch.Proposal{
		Path:      productmatch.PathSimilarity,
		ProductID: 100,
		Score:     0.95,
	}, polPackReq)

	if vPackUnresolved.State != matchflow.StateNeedsReview {
		t.Errorf("expected StateNeedsReview for pack_unresolved, got %v", vPackUnresolved.State)
	}
	if !hasReason(vPackUnresolved.Reasons, productmatch.ReasonPackUnresolved) {
		t.Errorf("expected ReasonPackUnresolved")
	}

	// Reviewer path confirmation
	vConfirmed := gate.Evaluate(rowFactsNoPack, productmatch.Proposal{
		Path:      productmatch.PathReviewer,
		ProductID: 100,
	}, productmatch.DefaultPolicy())

	if vConfirmed.State != matchflow.StateConfirmed {
		t.Errorf("expected StateConfirmed for reviewer proposal, got %v", vConfirmed.State)
	}
}
