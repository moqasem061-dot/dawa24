package smartorder

import (
	"sort"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Supplier selection: strict priority within a tolerance band.
//
// The buyer orders the criteria they care about. The highest-priority one
// decides — but only among candidates that are not unreasonably more expensive
// than the cheapest eligible offer. That qualifier is the whole design.
//
// Strict priority alone is too rigid: "prefer suppliers I follow" would pick a
// followed supplier at any price, and a pharmacy that discovers it paid 40% over
// the market stops trusting the tool immediately. A weighted score is too opaque:
// the buyer asks "why this supplier?" and the honest answer is a number nobody
// can argue with. The band keeps the preference meaningful and bounded, and every
// line can still state in one sentence what decided it.
//
// All comparisons run on net price per unit *after* discount, in minor units.
// Comparing list prices would let a supplier win on a headline number while
// costing more, which is precisely the manipulation the buyer is trying to avoid.

// Select chooses one eligible candidate for a line under the given config.
//
// It is pure and deterministic: the same inputs produce the same selection, in
// any order, every time. Re-running an unchanged file must not quietly swap
// suppliers, or the buyer cannot tell a real change from noise.
func Select(cfg *Config, lineID int64, candidates []Candidate) (*Selection, bool) {
	eligible := eligibleOf(candidates)
	if len(eligible) == 0 {
		return nil, false
	}

	if len(eligible) == 1 {
		return &Selection{
			LineID:         lineID,
			OrganizationID: eligible[0].OrganizationID,
			CandidateID:    eligible[0].ID,
			DecidedBy:      DecidedOnlyCandidate,
		}, true
	}

	if cfg == nil {
		cfg = &Config{Criteria: DefaultCriteria}
	}
	criteria := cfg.Criteria
	if len(criteria) == 0 {
		criteria = DefaultCriteria
	}

	cheapest := cheapestOf(eligible)
	band := toleranceBand(cheapest.NetUnitPrice, cfg.ToleranceBps())
	affordable := withinBand(eligible, band)

	// withinBand always retains the cheapest candidate, so affordable is never
	// empty. Guarding anyway: a future change to the band arithmetic must not be
	// able to produce a line with no supplier while eligible offers exist.
	if len(affordable) == 0 {
		affordable = []Candidate{cheapest}
	}

	// skipped is the first supplier a criterion would have chosen if price were
	// no object, and the band rejected. It is carried across the loop rather
	// than checked only against the deciding criterion, because the case that
	// matters most is the band eliminating a *higher-priority* criterion
	// outright: "prefer my suppliers" finds nobody affordable, price decides
	// instead, and the buyer needs to be told that is what happened.
	var skipped *Candidate

	for _, criterion := range criteria {
		unbounded, hasUnbounded := bestUnder(criterion, eligible)
		winner, ok := bestUnder(criterion, affordable)
		if !ok {
			// The criterion had an answer, but not one inside the band.
			if hasUnbounded && skipped == nil {
				c := unbounded
				skipped = &c
			}
			continue
		}
		if hasUnbounded && unbounded.ID != winner.ID && skipped == nil {
			c := unbounded
			skipped = &c
		}
		return withSkip(&Selection{
			LineID:         lineID,
			OrganizationID: winner.OrganizationID,
			CandidateID:    winner.ID,
			DecidedBy:      decidedBy(criterion),
		}, skipped, cheapest), true
	}

	// Every enabled criterion abstained — possible when the only one enabled is
	// FollowedSuppliers and the buyer follows nobody who sells this affordably.
	// Cheapest wins, and the line says the default decided it.
	fallback := cheapestOf(affordable)
	return withSkip(&Selection{
		LineID:         lineID,
		OrganizationID: fallback.OrganizationID,
		CandidateID:    fallback.ID,
		DecidedBy:      DecidedDefault,
	}, skipped, cheapest), true
}

// withSkip records the supplier the tolerance band passed over, if any.
//
// Naming it, and saying by how much it missed, is what turns "the system chose
// someone else" into a rule the buyer can reason about and adjust.
func withSkip(sel *Selection, skipped *Candidate, cheapest Candidate) *Selection {
	if skipped == nil || skipped.ID == sel.CandidateID {
		return sel
	}
	excess := excessPercent(cheapest.NetUnitPrice, skipped.NetUnitPrice)
	id := skipped.ID
	sel.ToleranceApplied = true
	sel.SkippedCandidateID = &id
	sel.SkippedExcessPct = &excess
	return sel
}

// eligibleOf keeps the offers a buyer may actually purchase.
func eligibleOf(candidates []Candidate) []Candidate {
	out := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Eligible {
			out = append(out, c)
		}
	}
	return out
}

// toleranceBand is the highest net unit price a preferred supplier may charge.
//
// Computed in minor units off the cheapest offer, so the band is exact and no
// float ever touches a price.
func toleranceBand(cheapest money.Amount, toleranceBps int64) money.Amount {
	if toleranceBps <= 0 {
		return cheapest
	}
	margin := cheapest.ApplyPercent(toleranceBps)
	band, err := cheapest.Add(margin)
	if err != nil {
		// Add only fails on overflow, unreachable for a unit price. Falling back
		// to the cheapest is the conservative direction: it narrows the band
		// rather than admitting an arbitrarily expensive supplier.
		return cheapest
	}
	return band
}

// withinBand keeps candidates at or under the band. The cheapest is always in it.
func withinBand(candidates []Candidate, band money.Amount) []Candidate {
	out := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.NetUnitPrice.Minor() <= band.Minor() {
			out = append(out, c)
		}
	}
	return out
}

// cheapestOf returns the lowest net unit price, ties broken deterministically.
func cheapestOf(candidates []Candidate) Candidate {
	best := candidates[0]
	for _, c := range candidates[1:] {
		if lessByTieBreak(c, best) {
			best = c
		}
	}
	return best
}

// bestUnder returns the winner for one criterion, or false when the criterion
// has nothing to say about this candidate set.
//
// Only FollowedSuppliers can abstain: price and discount always have an answer.
func bestUnder(criterion Criterion, candidates []Candidate) (Candidate, bool) {
	switch criterion {
	case CriterionLowestPrice:
		return cheapestOf(candidates), true

	case CriterionHighestDiscount:
		best := candidates[0]
		for _, c := range candidates[1:] {
			if c.DiscountBps > best.DiscountBps {
				best = c
				continue
			}
			if c.DiscountBps == best.DiscountBps && lessByTieBreak(c, best) {
				best = c
			}
		}
		return best, true

	case CriterionFollowedSuppliers:
		followed := make([]Candidate, 0, len(candidates))
		for _, c := range candidates {
			if c.IsFollowed {
				followed = append(followed, c)
			}
		}
		if len(followed) == 0 {
			return Candidate{}, false
		}
		return cheapestOf(followed), true
	}
	return Candidate{}, false
}

// lessByTieBreak is the total ordering that makes selection repeatable.
//
// Cheaper, then bigger discount, then followed, then lowest vendor id. The last
// key is arbitrary but *total*: without it two identical offers would be ordered
// by whatever the database returned that day, and re-running the same file would
// silently swap suppliers.
func lessByTieBreak(a, b Candidate) bool {
	if a.NetUnitPrice.Minor() != b.NetUnitPrice.Minor() {
		return a.NetUnitPrice.Minor() < b.NetUnitPrice.Minor()
	}
	if a.DiscountBps != b.DiscountBps {
		return a.DiscountBps > b.DiscountBps
	}
	if a.IsFollowed != b.IsFollowed {
		return a.IsFollowed
	}
	if a.VendorOrgID != b.VendorOrgID {
		return a.VendorOrgID < b.VendorOrgID
	}
	return a.ID < b.ID
}

// excessPercent is how far above the cheapest offer a skipped supplier sat.
//
// Reported to the buyer, so it is a display figure, not money: it never feeds
// back into a price.
func excessPercent(cheapest, skipped money.Amount) float64 {
	if cheapest.Minor() == 0 {
		return 0
	}
	diff := skipped.Minor() - cheapest.Minor()
	return float64(diff) * 100 / float64(cheapest.Minor())
}

func decidedBy(c Criterion) DecidedBy {
	switch c {
	case CriterionLowestPrice:
		return DecidedLowestPrice
	case CriterionHighestDiscount:
		return DecidedHighestDiscount
	case CriterionFollowedSuppliers:
		return DecidedFollowedSuppliers
	}
	return DecidedDefault
}

// SortCandidatesForDisplay orders candidates the way the buyer should read them:
// eligible first, then cheapest, with ineligible offers grouped after so the
// "why not that supplier" answer is right there rather than on another screen.
func SortCandidatesForDisplay(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Eligible != candidates[j].Eligible {
			return candidates[i].Eligible
		}
		return lessByTieBreak(candidates[i], candidates[j])
	})
}

// LineNet is the line's contribution to the order total.
//
// Quantity is a whole number of sellable units by the time it reaches here;
// MulInt keeps the arithmetic exact rather than multiplying by a float and
// rounding at the end, which is how an invoice ends up a piastre out.
func LineNet(unit money.Amount, qty float64) (money.Amount, error) {
	if qty <= 0 {
		return money.Amount{}, nil
	}
	return unit.MulInt(int64(qty))
}
