package smartorder

import (
	"math/rand"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// cand builds an eligible candidate. Net price is given in EGP for readability;
// discount in basis points, matching money.ApplyPercent.
func cand(id, vendorOrg int64, netEGP string, discountBps int64, followed bool) Candidate {
	return Candidate{
		ID:             id,
		LineID:         1,
		OrganizationID: 50, // the buyer
		VendorOrgID:    vendorOrg,
		VariantID:      id * 10,
		NetUnitPrice:   money.MustParse(netEGP),
		DiscountBps:    discountBps,
		IsFollowed:     followed,
		MinOrderQty:    1,
		StockQty:       100,
		Eligible:       true,
	}
}

func cfgWith(criteria []Criterion, tolerancePct float64) *Config {
	return &Config{
		OrganizationID: 50,
		Criteria:       criteria,
		TolerancePct:   tolerancePct,
	}
}

func TestSelectPicksLowestNetPrice(t *testing.T) {
	// Spec US1 scenario 4: three vendors, Lowest Price on top.
	candidates := []Candidate{
		cand(1, 101, "120.00", 1000, false),
		cand(2, 102, "100.00", 500, false),
		cand(3, 103, "110.00", 2000, false),
	}
	sel, ok := Select(cfgWith([]Criterion{CriterionLowestPrice}, 5), 1, candidates)
	if !ok {
		t.Fatal("expected a selection")
	}
	if sel.CandidateID != 2 {
		t.Fatalf("expected candidate 2 (100.00), got %d", sel.CandidateID)
	}
	if sel.DecidedBy != DecidedLowestPrice {
		t.Fatalf("expected decided_by lowest_price, got %s", sel.DecidedBy)
	}
}

func TestSelectComparesNetNotListPrice(t *testing.T) {
	// FR-033. A supplier with the lower headline price but a worse discount must
	// not win: the net is what the pharmacy pays.
	cheapList := cand(1, 101, "95.00", 0, false)
	betterNet := cand(2, 102, "90.00", 3000, false)
	sel, ok := Select(cfgWith([]Criterion{CriterionLowestPrice}, 0), 1, []Candidate{cheapList, betterNet})
	if !ok || sel.CandidateID != 2 {
		t.Fatalf("expected the lower net price to win, got %+v", sel)
	}
}

func TestToleranceBandRejectsExpensiveFollowedSupplier(t *testing.T) {
	// Spec US3 scenario 4: followed supplier at 130, unfollowed at 100,
	// tolerance 5%. The followed one is 30% over and must be passed over.
	followed := cand(1, 101, "130.00", 0, true)
	cheap := cand(2, 102, "100.00", 0, false)

	sel, ok := Select(cfgWith([]Criterion{CriterionFollowedSuppliers, CriterionLowestPrice}, 5), 1,
		[]Candidate{followed, cheap})
	if !ok {
		t.Fatal("expected a selection")
	}
	if sel.CandidateID != 2 {
		t.Fatalf("expected the 100.00 supplier, got candidate %d", sel.CandidateID)
	}
	if !sel.ToleranceApplied {
		t.Fatal("expected tolerance_applied to be recorded")
	}
	if sel.SkippedCandidateID == nil || *sel.SkippedCandidateID != 1 {
		t.Fatalf("expected the followed supplier to be named as skipped, got %v", sel.SkippedCandidateID)
	}
	if sel.SkippedExcessPct == nil || *sel.SkippedExcessPct != 30 {
		t.Fatalf("expected 30%% excess, got %v", sel.SkippedExcessPct)
	}
}

func TestToleranceBandAdmitsFollowedSupplierInsideIt(t *testing.T) {
	// Spec US3 scenario 5: followed at 103 against cheapest 100, tolerance 5%.
	// 103 is inside the band, so the preference stands.
	followed := cand(1, 101, "103.00", 0, true)
	cheap := cand(2, 102, "100.00", 0, false)

	sel, ok := Select(cfgWith([]Criterion{CriterionFollowedSuppliers, CriterionLowestPrice}, 5), 1,
		[]Candidate{followed, cheap})
	if !ok || sel.CandidateID != 1 {
		t.Fatalf("expected the followed supplier at 103.00 to win, got %+v", sel)
	}
	if sel.DecidedBy != DecidedFollowedSuppliers {
		t.Fatalf("expected decided_by followed_suppliers, got %s", sel.DecidedBy)
	}
	if sel.ToleranceApplied {
		t.Fatal("tolerance should not be reported when it changed nothing")
	}
}

func TestWideToleranceLetsFollowedSupplierWin(t *testing.T) {
	// The same pair at 50% tolerance: the buyer has said they accept the premium.
	followed := cand(1, 101, "130.00", 0, true)
	cheap := cand(2, 102, "100.00", 0, false)

	sel, ok := Select(cfgWith([]Criterion{CriterionFollowedSuppliers, CriterionLowestPrice}, 50), 1,
		[]Candidate{followed, cheap})
	if !ok || sel.CandidateID != 1 {
		t.Fatalf("expected the followed supplier to win at 50%% tolerance, got %+v", sel)
	}
}

func TestCriterionFallsThroughWhenItCannotDecide(t *testing.T) {
	// Spec US3 scenario 2 / FR-031: nobody followed sells this, so the next
	// criterion decides and the line is not left unselected.
	a := cand(1, 101, "120.00", 500, false)
	b := cand(2, 102, "110.00", 3000, false)

	sel, ok := Select(cfgWith([]Criterion{CriterionFollowedSuppliers, CriterionHighestDiscount}, 100), 1,
		[]Candidate{a, b})
	if !ok {
		t.Fatal("a line with eligible suppliers must never be left unselected")
	}
	if sel.DecidedBy != DecidedHighestDiscount || sel.CandidateID != 2 {
		t.Fatalf("expected highest discount to decide, got %+v", sel)
	}
}

func TestOnlyFollowedCriterionWithNoFollowedSupplierFallsBackToDefault(t *testing.T) {
	a := cand(1, 101, "120.00", 0, false)
	b := cand(2, 102, "110.00", 0, false)

	sel, ok := Select(cfgWith([]Criterion{CriterionFollowedSuppliers}, 100), 1, []Candidate{a, b})
	if !ok {
		t.Fatal("expected a fallback selection")
	}
	if sel.DecidedBy != DecidedDefault {
		t.Fatalf("expected decided_by default, got %s", sel.DecidedBy)
	}
	if sel.CandidateID != 2 {
		t.Fatalf("expected the cheapest as fallback, got %d", sel.CandidateID)
	}
}

func TestNoEligibleCandidatesYieldsNoSelection(t *testing.T) {
	blocked := cand(1, 101, "100.00", 0, false)
	blocked.Eligible = false
	blocked.IneligibleReason = ReasonCoverage

	if _, ok := Select(cfgWith(nil, 5), 1, []Candidate{blocked}); ok {
		t.Fatal("an ineligible candidate must never be selected")
	}
}

func TestSingleCandidateIsReportedAsSuch(t *testing.T) {
	sel, ok := Select(cfgWith([]Criterion{CriterionLowestPrice}, 5), 1,
		[]Candidate{cand(7, 101, "88.00", 0, false)})
	if !ok || sel.DecidedBy != DecidedOnlyCandidate {
		t.Fatalf("expected only_candidate, got %+v", sel)
	}
}

// FR-034 / SC gate: re-running the same file against unchanged data must select
// the same suppliers. Shuffling the input is how a dependence on database row
// order would show up.
func TestSelectionIsDeterministicUnderShuffling(t *testing.T) {
	base := []Candidate{
		cand(1, 101, "100.00", 1000, false),
		cand(2, 102, "100.00", 1000, false), // exact tie with 1
		cand(3, 103, "100.00", 1000, true),  // tie, but followed
		cand(4, 104, "104.00", 2500, true),
		cand(5, 105, "100.50", 1000, false),
	}
	cfg := cfgWith([]Criterion{CriterionHighestDiscount, CriterionLowestPrice}, 5)

	shuffled := make([]Candidate, len(base))
	copy(shuffled, base)
	first, ok := Select(cfg, 1, shuffled)
	if !ok {
		t.Fatal("expected a selection")
	}

	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 1000; i++ {
		rng.Shuffle(len(shuffled), func(a, b int) {
			shuffled[a], shuffled[b] = shuffled[b], shuffled[a]
		})
		got, ok := Select(cfg, 1, shuffled)
		if !ok {
			t.Fatalf("iteration %d: expected a selection", i)
		}
		if got.CandidateID != first.CandidateID {
			t.Fatalf("iteration %d: selection changed with input order: %d then %d",
				i, first.CandidateID, got.CandidateID)
		}
		if got.DecidedBy != first.DecidedBy {
			t.Fatalf("iteration %d: reason changed: %s then %s", i, first.DecidedBy, got.DecidedBy)
		}
	}
}

func TestTieBreakPrefersFollowedThenLowestVendorID(t *testing.T) {
	// Two identical offers; the followed one wins. Then two identical unfollowed
	// offers; the lower vendor id wins, which is arbitrary but total.
	a := cand(1, 200, "50.00", 0, false)
	b := cand(2, 100, "50.00", 0, true)
	if !lessByTieBreak(b, a) {
		t.Fatal("a followed supplier should outrank an identical unfollowed one")
	}

	c := cand(3, 300, "50.00", 0, false)
	d := cand(4, 150, "50.00", 0, false)
	if !lessByTieBreak(d, c) {
		t.Fatal("with everything else equal the lower vendor id must win, for repeatability")
	}
}

func TestZeroToleranceMeansCheapestOnly(t *testing.T) {
	followed := cand(1, 101, "100.01", 0, true)
	cheap := cand(2, 102, "100.00", 0, false)

	sel, ok := Select(cfgWith([]Criterion{CriterionFollowedSuppliers, CriterionLowestPrice}, 0), 1,
		[]Candidate{followed, cheap})
	if !ok || sel.CandidateID != 2 {
		t.Fatalf("at zero tolerance only the cheapest is admissible, got %+v", sel)
	}
}

func TestLineNetIsExact(t *testing.T) {
	// AGENTS.md rule 1: exact-value assertions, no approximate comparison.
	unit := money.MustParse("33.33")
	net, err := LineNet(unit, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if net.String() != "99.99" {
		t.Fatalf("expected exactly 99.99, got %s", net.String())
	}
	if net.Minor() != 9999 {
		t.Fatalf("expected 9999 minor units, got %d", net.Minor())
	}
}

func TestLineNetOfZeroQuantityIsZero(t *testing.T) {
	net, err := LineNet(money.MustParse("42.00"), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !net.IsZero() {
		t.Fatalf("a zero-quantity line must contribute nothing, got %s", net.String())
	}
}

func TestBudgetStatusReportsButNeverBlocks(t *testing.T) {
	budget := money.MustParse("1000.00")
	cfg := &Config{MaxBudget: &budget}

	exceeded, overage, has := cfg.BudgetStatus(money.MustParse("1250.50"))
	if !has || !exceeded {
		t.Fatal("expected the budget to be reported as exceeded")
	}
	if overage.String() != "250.50" {
		t.Fatalf("expected an overage of exactly 250.50, got %s", overage.String())
	}

	exceeded, _, _ = cfg.BudgetStatus(money.MustParse("1000.00"))
	if exceeded {
		t.Fatal("a total equal to the budget is not over it")
	}

	noBudget := &Config{}
	if _, _, has := noBudget.BudgetStatus(money.MustParse("9999.00")); has {
		t.Fatal("no budget set means no budget status")
	}
}
