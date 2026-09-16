package ui

import (
	"math"
	"sort"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// How the supplier offers under a product are ordered, and why.
//
// Split from offers_storefront.go, which builds the offers; this file decides
// what order a pharmacy reads them in.

// distanceBand buckets a distance into whole kilometres so two suppliers within
// the same kilometre are ordered by price rather than by GPS noise.
//
// The band is what makes the comparison transitive. Comparing raw distances
// with a "only if they differ by more than 1 km" guard is not: with offers at
// 0.0 km, 0.6 km and 1.4 km, the first and second tie on distance, the second
// and third tie too, but the first and third do not — and a comparator that
// contradicts itself lets sort.SliceStable return an arbitrary permutation, one
// that can scatter the in-stock offers this function put first. Bucketing
// compares equal-or-not consistently for every pair.
//
// An unknown distance (no coordinates on either side) sorts last rather than
// first, so a supplier we cannot place never outranks one we can.
func distanceBand(km float64) int {
	if km <= 0 {
		return math.MaxInt32
	}
	return int(km)
}

// sortSupplierOffers puts the offers a pharmacy can actually act on at the top
// of the page, in the order a buyer reads them:
//
//  1. can be added to the cart right now,
//  2. covered by the supplier's delivery area,
//  3. in stock,
//  4. nearest first,
//  5. cheapest first.
//
// Steps 3 and 4 are the "الأقرب جغرافيا، والمتوفر أولاً" rule: availability
// outranks proximity, and both outrank price.
func sortSupplierOffers(offers []pages.SupplierOffer) {
	sort.SliceStable(offers, func(i, j int) bool {
		a, b := offers[i], offers[j]
		if a.CanAddToCart != b.CanAddToCart {
			return a.CanAddToCart
		}
		if a.IsCovered != b.IsCovered {
			return a.IsCovered
		}
		if (a.AvailableStock > 0) != (b.AvailableStock > 0) {
			return a.AvailableStock > 0
		}
		if ba, bb := distanceBand(a.DistanceKM), distanceBand(b.DistanceKM); ba != bb {
			return ba < bb
		}
		return a.Price.Minor() < b.Price.Minor()
	})
}

// promoteFocusedOffer moves the offer for focusVariantID to the front of the
// group of equally-ranked offers it already sits in, leaving every better-ranked
// offer above it. A pharmacy that followed a link to one supplier's listing sees
// that supplier first among the ones it can actually order from, and never sees
// an unorderable offer promoted over an orderable one.
func promoteFocusedOffer(offers []pages.SupplierOffer, focusVariantID int64) {
	if focusVariantID <= 0 || len(offers) < 2 {
		return
	}
	at := -1
	for i := range offers {
		if offers[i].VariantID == focusVariantID {
			at = i
			break
		}
	}
	if at <= 0 {
		return
	}
	// Walk back over the offers this one ties with on every ranking key.
	dest := at
	for dest > 0 && sameOfferRank(offers[dest-1], offers[at]) {
		dest--
	}
	if dest == at {
		return
	}
	focused := offers[at]
	copy(offers[dest+1:at+1], offers[dest:at])
	offers[dest] = focused
}

// sameOfferRank reports whether two offers are indistinguishable to
// sortSupplierOffers, i.e. neither would sort before the other.
func sameOfferRank(a, b pages.SupplierOffer) bool {
	return a.CanAddToCart == b.CanAddToCart &&
		a.IsCovered == b.IsCovered &&
		(a.AvailableStock > 0) == (b.AvailableStock > 0) &&
		distanceBand(a.DistanceKM) == distanceBand(b.DistanceKM)
}
