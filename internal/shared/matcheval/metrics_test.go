package matcheval

import (
	"math"
	"testing"
)

func int64Ptr(v int64) *int64 {
	return &v
}

func TestScore_HandComputed10Cases(t *testing.T) {
	cases := []Case{
		// Case 1: Resolvable, Target 101, Accepted 101 (Correct)
		{
			ID:       "c-1",
			Workflow: []string{"vendor"},
			Language: "ar",
			Label:    Label{Resolvable: true, Targets: []int64{101}, Expect: "accept"},
		},
		// Case 2: Resolvable, Target 102, Accepted 102 (Correct)
		{
			ID:       "c-2",
			Workflow: []string{"vendor"},
			Language: "ar",
			Label:    Label{Resolvable: true, Targets: []int64{102}, Expect: "accept"},
		},
		// Case 3: Resolvable, Target 103, Accepted 103 (Correct)
		{
			ID:       "c-3",
			Workflow: []string{"savings"},
			Language: "en",
			Label:    Label{Resolvable: true, Targets: []int64{103}, Expect: "accept"},
		},
		// Case 4: Resolvable, Target 104, Accepted 999 (Wrong acceptance)
		{
			ID:       "c-4",
			Workflow: []string{"savings"},
			Language: "en",
			Label:    Label{Resolvable: true, Targets: []int64{104}, Expect: "accept"},
		},
		// Case 5: Resolvable, Target 105, Unaccepted, Top20 contains 105
		{
			ID:       "c-5",
			Workflow: []string{"vendor"},
			Language: "ar",
			Label:    Label{Resolvable: true, Targets: []int64{105}, Expect: "accept"},
		},
		// Case 6: Resolvable, Target 106, Unaccepted, Top20 does not contain 106
		{
			ID:       "c-6",
			Workflow: []string{"vendor"},
			Language: "ar",
			Label:    Label{Resolvable: true, Targets: []int64{106}, Expect: "accept"},
		},
		// Case 7: Absent, Correctly unaccepted
		{
			ID:       "c-7",
			Workflow: []string{"vendor"},
			Language: "ar",
			Label:    Label{Resolvable: false, Targets: nil, Expect: "abstain"},
		},
		// Case 8: Absent, Accepted 108 (Wrong acceptance of absent target)
		{
			ID:       "c-8",
			Workflow: []string{"vendor"},
			Language: "ar",
			Label:    Label{Resolvable: false, Targets: nil, Expect: "abstain"},
		},
		// Case 9: Resolvable, Target 109, Accepted 109 but with hard conflict
		{
			ID:       "c-9",
			Workflow: []string{"vendor"},
			Language: "ar",
			Label:    Label{Resolvable: true, Targets: []int64{109}, Expect: "accept"},
		},
		// Case 10: Disputed (must be excluded from evaluated counts)
		{
			ID:       "c-10",
			Workflow: []string{"vendor"},
			Language: "ar",
			Label:    Label{Resolvable: true, Targets: []int64{110}, Disputed: true},
		},
	}

	outcomes := []Outcome{
		{CaseID: "c-1", State: "accepted", MatchedProductID: int64Ptr(101), TopCandidates: []int64{101}},
		{CaseID: "c-2", State: "accepted", MatchedProductID: int64Ptr(102), TopCandidates: []int64{102}},
		{CaseID: "c-3", State: "accepted", MatchedProductID: int64Ptr(103), TopCandidates: []int64{103}},
		{CaseID: "c-4", State: "accepted", MatchedProductID: int64Ptr(999), TopCandidates: []int64{104, 999}},
		{CaseID: "c-5", State: "needs_review", TopCandidates: []int64{105}},
		{CaseID: "c-6", State: "needs_review", TopCandidates: []int64{200, 201}},
		{CaseID: "c-7", State: "needs_review", TopCandidates: nil},
		{CaseID: "c-8", State: "accepted", MatchedProductID: int64Ptr(108), TopCandidates: []int64{108}},
		{CaseID: "c-9", State: "accepted", MatchedProductID: int64Ptr(109), TopCandidates: []int64{109}, HardConflictViolations: []string{"strength_conflict"}},
		{CaseID: "c-10", State: "accepted", MatchedProductID: int64Ptr(110)},
	}

	report := Score(cases, outcomes)

	o := report.Overall
	if o.NCases != 9 {
		t.Errorf("NCases = %d, want 9", o.NCases)
	}
	if o.NDisputedExcluded != 1 {
		t.Errorf("NDisputedExcluded = %d, want 1", o.NDisputedExcluded)
	}
	if o.NResolvable != 7 {
		t.Errorf("NResolvable = %d, want 7", o.NResolvable)
	}
	if o.Accepted != 6 {
		t.Errorf("Accepted = %d, want 6", o.Accepted)
	}
	if o.AcceptedCorrect != 4 {
		t.Errorf("AcceptedCorrect = %d, want 4", o.AcceptedCorrect)
	}
	if o.AbsentOrAmbiguousAccepted != 1 {
		t.Errorf("AbsentOrAmbiguousAccepted = %d, want 1", o.AbsentOrAmbiguousAccepted)
	}

	// Precision = 4 / 6 = 0.6666667
	wantPrec := 4.0 / 6.0
	if math.Abs(o.Precision-wantPrec) > 1e-5 {
		t.Errorf("Precision = %f, want %f", o.Precision, wantPrec)
	}

	// RecallAt20 = 6 / 7 (cases 1, 2, 3, 4, 5, 9 have target in top 20)
	wantRecall := 6.0 / 7.0
	if math.Abs(o.RecallAt20-wantRecall) > 1e-5 {
		t.Errorf("RecallAt20 = %f, want %f", o.RecallAt20, wantRecall)
	}

	// CoverageResolvable = 4 / 7
	wantCovRes := 4.0 / 7.0
	if math.Abs(o.CoverageResolvable-wantCovRes) > 1e-5 {
		t.Errorf("CoverageResolvable = %f, want %f", o.CoverageResolvable, wantCovRes)
	}

	// CoverageAllRows = 4 / 9
	wantCovAll := 4.0 / 9.0
	if math.Abs(o.CoverageAllRows-wantCovAll) > 1e-5 {
		t.Errorf("CoverageAllRows = %f, want %f", o.CoverageAllRows, wantCovAll)
	}

	// ReviewWorkload = (9 - 6) / 9 = 3 / 9
	wantWorkload := 3.0 / 9.0
	if math.Abs(o.ReviewWorkload-wantWorkload) > 1e-5 {
		t.Errorf("ReviewWorkload = %f, want %f", o.ReviewWorkload, wantWorkload)
	}

	// HardConflictViolations = 1
	if len(o.HardConflictViolations) != 1 {
		t.Fatalf("HardConflictViolations = %d, want 1", len(o.HardConflictViolations))
	}
	if o.HardConflictViolations[0].CaseID != "c-9" {
		t.Errorf("HardConflictViolation CaseID = %s, want c-9", o.HardConflictViolations[0].CaseID)
	}

	// Check Gates:
	// SC-001: insufficient_sample because Accepted (6) < 1000
	if report.Gates["SC-001"] != GateInsufficientSample {
		t.Errorf("Gate SC-001 = %v, want %v", report.Gates["SC-001"], GateInsufficientSample)
	}
	// SC-002: fail because hard conflict was present
	if report.Gates["SC-002"] != GateFail {
		t.Errorf("Gate SC-002 = %v, want %v", report.Gates["SC-002"], GateFail)
	}
	// SC-003: insufficient_sample because NResolvable (7) < 100
	if report.Gates["SC-003"] != GateInsufficientSample {
		t.Errorf("Gate SC-003 = %v, want %v", report.Gates["SC-003"], GateInsufficientSample)
	}
	// SC-004: insufficient_sample because NResolvable (7) < 100
	if report.Gates["SC-004"] != GateInsufficientSample {
		t.Errorf("Gate SC-004 = %v, want %v", report.Gates["SC-004"], GateInsufficientSample)
	}
}

func TestScore_LargeSampleGates(t *testing.T) {
	// Generate 1200 cases to test sufficient sample thresholds
	cases := make([]Case, 1200)
	outcomes := make([]Outcome, 1200)

	for i := 0; i < 1200; i++ {
		targetID := int64(1000 + i)
		cases[i] = Case{
			ID:       "c-" + string(rune(i)),
			Workflow: []string{"vendor"},
			Language: "ar",
			Label:    Label{Resolvable: true, Targets: []int64{targetID}, Expect: "accept"},
		}
		// 99.8% precision, all in top 20, no hard conflicts
		matchedID := targetID
		if i == 0 || i == 1 {
			matchedID = 999999 // 2 errors out of 1200 = 99.83% precision
		}
		outcomes[i] = Outcome{
			CaseID:           cases[i].ID,
			State:            "accepted",
			MatchedProductID: &matchedID,
			TopCandidates:    []int64{targetID},
		}
	}

	report := Score(cases, outcomes)

	if report.Gates["SC-001"] != GatePass {
		t.Errorf("Gate SC-001 with 1200 cases = %v, want %v (Precision=%f, LB=%f)",
			report.Gates["SC-001"], GatePass, report.Overall.Precision, report.Overall.PrecisionLB95)
	}
	if report.Gates["SC-002"] != GatePass {
		t.Errorf("Gate SC-002 = %v, want %v", report.Gates["SC-002"], GatePass)
	}
	if report.Gates["SC-003"] != GatePass {
		t.Errorf("Gate SC-003 = %v, want %v", report.Gates["SC-003"], GatePass)
	}
	if report.Gates["SC-004"] != GatePass {
		t.Errorf("Gate SC-004 = %v, want %v", report.Gates["SC-004"], GatePass)
	}
}
