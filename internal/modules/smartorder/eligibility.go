package smartorder

// Offer eligibility.
//
// Purchase eligibility is decided by commerce.CheckAvailability via the
// AvailabilityGate. This file maps the commerce reason onto smart ordering's
// presentation types (IneligibleReason and Outcome) and owns the outcome
// severity ranking across candidates.

// Evaluate maps a commerce refusal Reason to whether the offer is orderable
// and its smartorder IneligibleReason.
func Evaluate(reason string) (bool, IneligibleReason) {
	if reason == "" {
		return true, ""
	}
	return false, EvaluateReason(reason)
}

// EvaluateReason maps a commerce Reason code to a smartorder IneligibleReason.
func EvaluateReason(reason string) IneligibleReason {
	switch reason {
	case "own_organization", "own_org":
		return ReasonOwnOrg
	case "vendor_invalid", "vendor_unapproved", "wrong_vendor", "variant_invalid", "variant_inactive", "inactive":
		return ReasonInactive
	case "branch_no_institutional_works", "branch_institutional_mismatch", "institutional":
		return ReasonInstitutional
	case "branch_no_location":
		return ReasonNoLocation
	case "not_covered", "coverage", "branch_invalid", "branch_not_owned":
		return ReasonCoverage
	case "out_of_stock", "insufficient_stock", "stock":
		return ReasonStock
	case "below_minimum", "min_qty":
		return ReasonMinQty
	case "quota_exhausted", "quota_exceeded", "quota":
		return ReasonQuota
	default:
		return IneligibleReason(reason)
	}
}

// OutcomeFor derives a line's outcome from the candidates found for it.
//
// When every candidate was rejected, the line reports the *least severe*
// obstacle across them — the one closest to being orderable. If one vendor is
// merely out of stock while three others are outside coverage, "out of stock" is
// the useful answer: it is the one a buyer can act on by waiting or by asking
// that vendor, whereas coverage is a fact about geography.
func OutcomeFor(matched bool, effectiveQty float64, candidates []Candidate) (Outcome, IneligibleReason) {
	if !matched {
		return OutcomeUnmatched, ""
	}
	if effectiveQty <= 0 {
		return OutcomeZeroQty, ""
	}
	if len(candidates) == 0 {
		return OutcomeNoSupplier, ""
	}

	for _, c := range candidates {
		if c.Eligible {
			return OutcomeOrdered, ""
		}
	}

	// Severity order, least severe first.
	ranked := []struct {
		reason  IneligibleReason
		outcome Outcome
	}{
		{ReasonMinQty, OutcomeBelowMinQty},
		{ReasonStock, OutcomeOutOfStock},
		{ReasonQuota, OutcomeQuotaBlocked},
		{ReasonNoLocation, OutcomeCoverageBlocked},
		{ReasonCoverage, OutcomeCoverageBlocked},
		{ReasonInstitutional, OutcomeInstitutionalBlocked},
		{ReasonInactive, OutcomeNoSupplier},
		{ReasonOwnOrg, OutcomeNoSupplier},
	}
	for _, r := range ranked {
		for _, c := range candidates {
			if c.IneligibleReason == r.reason {
				return r.outcome, r.reason
			}
		}
	}

	return OutcomeNoSupplier, ""
}

// CountByOutcome tallies lines for the results statistics.
func CountByOutcome(lines []*Line) Stats {
	var s Stats
	for _, l := range lines {
		s.TotalRows++
		if l.Matched() {
			s.MatchedRows++
		}
		switch l.Outcome {
		case OutcomeUnmatched:
			s.UnmatchedRows++
		case OutcomeNoSupplier:
			s.NoSupplierRows++
		case OutcomeCoverageBlocked:
			s.CoverageBlockedRows++
		case OutcomeInstitutionalBlocked:
			s.InstitutionalBlockedRows++
		case OutcomeBelowMinQty:
			s.BelowMinQtyRows++
		case OutcomeQuotaBlocked:
			s.QuotaBlockedRows++
		}
	}
	return s
}
