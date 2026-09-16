package org

import (
	"fmt"
	"sort"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// شرائح ورسوم التوصيل — turning a distance into a fee, and saying how.
//
// The matching used to be a single loop over whatever order the repository
// returned, with three fallbacks stacked after it. Four things were wrong with
// it and all four are visible to a pharmacy at checkout:
//
//   - The bands were never sorted, so "below the first band" meant "below
//     whichever band the database listed first". A vendor whose 0–5 km tier was
//     inserted second charged a pharmacy 300 metres away the 20–50 km fee.
//   - An inactive first band was still used as the below-range fallback.
//   - A gap between two bands — 0–5 km and 10–20 km, with nothing between —
//     fell through to bands[0], so 7 km was charged as if it were 300 metres.
//   - The caller, when it could not work out a distance at all, invented five
//     kilometres and looked that up. Five kilometres is a real band on most
//     vendors' tables, so an unknown distance quietly produced a confident and
//     arbitrary fee.
//
// A quote now says which of those situations it is in, so the cart can print
// "رسوم التوصيل 25.00 ج.م لمسافة 7.2 كم" when it knows and something honest
// when it does not.

// QuoteBasis is how a delivery fee was arrived at.
type QuoteBasis string

const (
	// BasisExact is a distance measured between two known points, falling
	// inside a declared band.
	BasisExact QuoteBasis = "exact"
	// BasisBelowRange is a distance shorter than the vendor's nearest band. The
	// nearest band's fee applies: a vendor who declares no tier below 5 km is
	// stating that everything under 5 km costs the 5 km fee.
	BasisBelowRange QuoteBasis = "below_range"
	// BasisAboveRange is a distance beyond the vendor's furthest band. The
	// furthest band's fee applies, and the coverage check — not this — decides
	// whether the vendor delivers there at all.
	BasisAboveRange QuoteBasis = "above_range"
	// BasisGap is a distance falling between two declared bands, which is a
	// hole in the vendor's own table. The next band up applies, because
	// charging the lower one would be charging for a shorter journey than the
	// one being made.
	BasisGap QuoteBasis = "gap"
	// BasisUnknownDistance means one of the two endpoints has no coordinates.
	// The cheapest band applies, and the quote says so: a guessed distance must
	// not be able to produce an expensive fee.
	BasisUnknownDistance QuoteBasis = "unknown_distance"
	// BasisNoBands means the vendor has declared no delivery pricing. The fee
	// is zero, which is what a vendor with no tiers has said.
	BasisNoBands QuoteBasis = "no_bands"
)

// DeliveryQuote is a fee and the reasoning behind it.
type DeliveryQuote struct {
	Fee money.Amount `json:"fee"`
	// DistanceMeters is the measured distance, or -1 when it is unknown.
	DistanceMeters int        `json:"distance_meters"`
	Basis          QuoteBasis `json:"basis"`
	// Band is the tier that produced the fee, if any.
	Band *DeliveryBand `json:"band,omitempty"`
}

// Known reports whether the quote rests on a measured distance.
func (q DeliveryQuote) Known() bool { return q.DistanceMeters >= 0 }

// DistanceKM renders the distance the way a cart states it.
func (q DeliveryQuote) DistanceKM() float64 {
	if q.DistanceMeters < 0 {
		return 0
	}
	return float64(q.DistanceMeters) / 1000
}

// UnknownDistance is what a caller passes when it could not measure one.
const UnknownDistance = -1

// QuoteDelivery matches a distance against a vendor's declared bands.
//
// It is a pure function of the bands and the distance, so the whole of the
// pricing rule is testable without a database and the cart, the checkout and
// the supplier page cannot disagree about what a delivery costs.
func QuoteDelivery(bands []*DeliveryBand, distanceMeters int) DeliveryQuote {
	active := make([]*DeliveryBand, 0, len(bands))
	for _, b := range bands {
		if b != nil && b.IsActive && b.ToMeters > b.FromMeters {
			active = append(active, b)
		}
	}
	if len(active) == 0 {
		return DeliveryQuote{Fee: money.Zero, DistanceMeters: distanceMeters, Basis: BasisNoBands}
	}

	// Order is the rule, not the repository's row order.
	sort.SliceStable(active, func(i, j int) bool {
		if active[i].FromMeters != active[j].FromMeters {
			return active[i].FromMeters < active[j].FromMeters
		}
		return active[i].ToMeters < active[j].ToMeters
	})

	if distanceMeters < 0 {
		// The cheapest tier. An unmeasurable distance must not be able to bill
		// a pharmacy for the longest journey the vendor prices.
		cheapest := active[0]
		for _, b := range active {
			if b.Fee.Minor() < cheapest.Fee.Minor() {
				cheapest = b
			}
		}
		return DeliveryQuote{
			Fee: cheapest.Fee, DistanceMeters: UnknownDistance,
			Basis: BasisUnknownDistance, Band: cheapest,
		}
	}

	for _, b := range active {
		if distanceMeters >= b.FromMeters && distanceMeters <= b.ToMeters {
			return DeliveryQuote{
				Fee: b.Fee, DistanceMeters: distanceMeters,
				Basis: BasisExact, Band: b,
			}
		}
	}

	first, last := active[0], active[len(active)-1]
	if distanceMeters < first.FromMeters {
		return DeliveryQuote{
			Fee: first.Fee, DistanceMeters: distanceMeters,
			Basis: BasisBelowRange, Band: first,
		}
	}
	if distanceMeters > last.ToMeters {
		return DeliveryQuote{
			Fee: last.Fee, DistanceMeters: distanceMeters,
			Basis: BasisAboveRange, Band: last,
		}
	}

	// A hole in the vendor's table. Charge the next tier up.
	for _, b := range active {
		if b.FromMeters > distanceMeters {
			return DeliveryQuote{
				Fee: b.Fee, DistanceMeters: distanceMeters,
				Basis: BasisGap, Band: b,
			}
		}
	}
	return DeliveryQuote{
		Fee: last.Fee, DistanceMeters: distanceMeters,
		Basis: BasisGap, Band: last,
	}
}

// ValidateDeliveryBands rejects a table that cannot price a delivery correctly.
//
// Overlaps are the one thing that must be refused rather than resolved: two
// bands covering 7 km with different fees have no correct answer, and picking
// one silently is how a pharmacy and a supplier come to disagree about an
// invoice. Gaps are permitted — QuoteDelivery charges the next tier up and says
// it did — because a vendor mid-way through building their table should not be
// blocked from saving it.
func ValidateDeliveryBands(bands []*DeliveryBand) error {
	active := make([]*DeliveryBand, 0, len(bands))
	for _, b := range bands {
		if b == nil || !b.IsActive {
			continue
		}
		if b.FromMeters < 0 || b.ToMeters <= b.FromMeters {
			return errInvalidBand(b)
		}
		if b.Fee.Minor() < 0 {
			return errInvalidBand(b)
		}
		active = append(active, b)
	}
	sort.SliceStable(active, func(i, j int) bool { return active[i].FromMeters < active[j].FromMeters })
	for i := 1; i < len(active); i++ {
		if active[i].FromMeters <= active[i-1].ToMeters {
			return errOverlappingBands(active[i-1], active[i])
		}
	}
	return nil
}

// errInvalidBand reports a tier whose own numbers do not make sense.
func errInvalidBand(b *DeliveryBand) error {
	return apperr.Validation("delivery.band.invalid",
		fmt.Sprintf("شريحة التوصيل من %d إلى %d متر غير صالحة: يجب أن تكون نهاية الشريحة أكبر من بدايتها والرسوم غير سالبة.",
			b.FromMeters, b.ToMeters), nil)
}

// errOverlappingBands reports two tiers that both price the same distance.
func errOverlappingBands(a, b *DeliveryBand) error {
	return apperr.Validation("delivery.band.overlap",
		fmt.Sprintf("شريحتا التوصيل %d–%d و %d–%d متر متداخلتان. لا يمكن أن تحمل المسافة الواحدة سعرين.",
			a.FromMeters, a.ToMeters, b.FromMeters, b.ToMeters), nil)
}
