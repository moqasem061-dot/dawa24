package smartorder

import "testing"

func TestEvaluateEligiblePasses(t *testing.T) {
	ok, reason := Evaluate("")
	if !ok {
		t.Fatalf("expected empty reason to be eligible, got %q", reason)
	}
	if reason != "" {
		t.Fatalf("expected empty reason, got %q", reason)
	}
}

func TestEvaluateRefusalReasons(t *testing.T) {
	cases := []struct {
		input string
		want  IneligibleReason
	}{
		{"own_organization", ReasonOwnOrg},
		{"own_org", ReasonOwnOrg},
		{"vendor_invalid", ReasonInactive},
		{"vendor_unapproved", ReasonInactive},
		{"variant_invalid", ReasonInactive},
		{"variant_inactive", ReasonInactive},
		{"branch_no_institutional_works", ReasonInstitutional},
		{"branch_institutional_mismatch", ReasonInstitutional},
		{"institutional", ReasonInstitutional},
		{"branch_no_location", ReasonNoLocation},
		{"not_covered", ReasonCoverage},
		{"coverage", ReasonCoverage},
		{"out_of_stock", ReasonStock},
		{"insufficient_stock", ReasonStock},
		{"stock", ReasonStock},
		{"below_minimum", ReasonMinQty},
		{"min_qty", ReasonMinQty},
		{"quota_exhausted", ReasonQuota},
		{"quota_exceeded", ReasonQuota},
		{"quota", ReasonQuota},
	}
	for _, tc := range cases {
		ok, got := Evaluate(tc.input)
		if ok {
			t.Errorf("reason %q: expected not ok", tc.input)
		}
		if got != tc.want {
			t.Errorf("reason %q: expected %q, got %q", tc.input, tc.want, got)
		}
	}
}

func TestOutcomeForUnmatchedLine(t *testing.T) {
	if got, _ := OutcomeFor(false, 5, nil); got != OutcomeUnmatched {
		t.Fatalf("expected unmatched, got %s", got)
	}
}

func TestOutcomeForZeroQuantityBeatsSupplierState(t *testing.T) {
	if got, _ := OutcomeFor(true, 0, []Candidate{{Eligible: true}}); got != OutcomeZeroQty {
		t.Fatalf("expected zero_qty, got %s", got)
	}
}

func TestOutcomeForNoCandidatesIsNoSupplier(t *testing.T) {
	if got, _ := OutcomeFor(true, 5, nil); got != OutcomeNoSupplier {
		t.Fatalf("expected no_supplier, got %s", got)
	}
}

func TestOutcomeForAnyEligibleCandidateIsOrdered(t *testing.T) {
	candidates := []Candidate{
		{Eligible: false, IneligibleReason: ReasonCoverage},
		{Eligible: true},
	}
	if got, _ := OutcomeFor(true, 5, candidates); got != OutcomeOrdered {
		t.Fatalf("one eligible candidate is enough, got %s", got)
	}
}

func TestOutcomeForQuotaBlocked(t *testing.T) {
	candidates := []Candidate{
		{Eligible: false, IneligibleReason: ReasonQuota},
	}
	got, reason := OutcomeFor(true, 5, candidates)
	if got != OutcomeQuotaBlocked {
		t.Fatalf("expected quota_blocked, got %s", got)
	}
	if reason != ReasonQuota {
		t.Fatalf("expected reason quota, got %s", reason)
	}
}

func TestOutcomeForNoLocation(t *testing.T) {
	candidates := []Candidate{
		{Eligible: false, IneligibleReason: ReasonNoLocation},
	}
	got, reason := OutcomeFor(true, 5, candidates)
	if got != OutcomeCoverageBlocked {
		t.Fatalf("expected coverage_blocked, got %s", got)
	}
	if reason != ReasonNoLocation {
		t.Fatalf("expected reason branch_no_location, got %s", reason)
	}
}

func TestOutcomeForReportsLeastSevereObstacle(t *testing.T) {
	candidates := []Candidate{
		{Eligible: false, IneligibleReason: ReasonInstitutional},
		{Eligible: false, IneligibleReason: ReasonCoverage},
		{Eligible: false, IneligibleReason: ReasonQuota},
		{Eligible: false, IneligibleReason: ReasonStock},
	}
	got, reason := OutcomeFor(true, 5, candidates)
	if got != OutcomeOutOfStock {
		t.Fatalf("expected out_of_stock as more actionable than quota/coverage, got %s", got)
	}
	if reason != ReasonStock {
		t.Fatalf("expected reason stock, got %q", reason)
	}

	// Quota is more actionable than coverage
	quotaAndCov := []Candidate{
		{Eligible: false, IneligibleReason: ReasonCoverage},
		{Eligible: false, IneligibleReason: ReasonQuota},
	}
	got2, reason2 := OutcomeFor(true, 5, quotaAndCov)
	if got2 != OutcomeQuotaBlocked {
		t.Fatalf("expected quota_blocked, got %s", got2)
	}
	if reason2 != ReasonQuota {
		t.Fatalf("expected reason quota, got %q", reason2)
	}
}

func TestOutcomeForDistinguishesCoverageFromInstitutional(t *testing.T) {
	coverage := []Candidate{{Eligible: false, IneligibleReason: ReasonCoverage}}
	if got, _ := OutcomeFor(true, 5, coverage); got != OutcomeCoverageBlocked {
		t.Fatalf("expected coverage_blocked, got %s", got)
	}

	institutional := []Candidate{{Eligible: false, IneligibleReason: ReasonInstitutional}}
	if got, _ := OutcomeFor(true, 5, institutional); got != OutcomeInstitutionalBlocked {
		t.Fatalf("expected institutional_blocked, got %s", got)
	}
}

func TestCountByOutcome(t *testing.T) {
	pid := int64(1)
	lines := []*Line{
		{MatchedProductID: &pid, Outcome: OutcomeOrdered},
		{MatchedProductID: &pid, Outcome: OutcomeCoverageBlocked},
		{MatchedProductID: &pid, Outcome: OutcomeInstitutionalBlocked},
		{MatchedProductID: &pid, Outcome: OutcomeNoSupplier},
		{Outcome: OutcomeUnmatched},
		{MatchedProductID: &pid, Outcome: OutcomeBelowMinQty},
		{MatchedProductID: &pid, Outcome: OutcomeQuotaBlocked},
	}
	s := CountByOutcome(lines)

	if s.TotalRows != 7 {
		t.Errorf("total: expected 7, got %d", s.TotalRows)
	}
	if s.MatchedRows != 6 {
		t.Errorf("matched: expected 6, got %d", s.MatchedRows)
	}
	if s.UnmatchedRows != 1 {
		t.Errorf("unmatched: expected 1, got %d", s.UnmatchedRows)
	}
	if s.CoverageBlockedRows != 1 {
		t.Errorf("coverage: expected 1, got %d", s.CoverageBlockedRows)
	}
	if s.InstitutionalBlockedRows != 1 {
		t.Errorf("institutional: expected 1, got %d", s.InstitutionalBlockedRows)
	}
	if s.NoSupplierRows != 1 {
		t.Errorf("no supplier: expected 1, got %d", s.NoSupplierRows)
	}
	if s.BelowMinQtyRows != 1 {
		t.Errorf("below min qty: expected 1, got %d", s.BelowMinQtyRows)
	}
	if s.QuotaBlockedRows != 1 {
		t.Errorf("quota blocked: expected 1, got %d", s.QuotaBlockedRows)
	}
}
