package ui

import (
	"math/rand"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func offer(variantID int64, canCart, covered bool, stock int, km float64, priceMinor int64) pages.SupplierOffer {
	return pages.SupplierOffer{
		VariantID:      variantID,
		CanAddToCart:   canCart,
		IsCovered:      covered,
		AvailableStock: stock,
		DistanceKM:     km,
		Price:          money.FromMinor(priceMinor),
	}
}

func ids(offers []pages.SupplierOffer) []int64 {
	out := make([]int64, len(offers))
	for i := range offers {
		out[i] = offers[i].VariantID
	}
	return out
}

func equalIDs(got []int64, want ...int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// The order a pharmacy reads: orderable, covered, in stock, nearest, cheapest.
func TestSortSupplierOffersRanksAvailabilityThenProximity(t *testing.T) {
	offers := []pages.SupplierOffer{
		offer(1, false, false, 0, 1, 1000), // unreachable, cheapest, nearest
		offer(2, true, true, 0, 2, 1100),   // covered but out of stock
		offer(3, true, true, 40, 90, 3000), // in stock, far, dearest
		offer(4, true, true, 25, 5, 2500),  // in stock, near
		offer(5, false, true, 10, 1, 900),  // covered, in stock, not orderable
	}
	sortSupplierOffers(offers)

	if got := ids(offers); !equalIDs(got, 4, 3, 2, 5, 1) {
		t.Fatalf("offers ordered %v, want [4 3 2 5 1]", got)
	}
}

// An offer whose distance is unknown must not outrank one we can place.
func TestSortSupplierOffersPutsUnknownDistanceLast(t *testing.T) {
	offers := []pages.SupplierOffer{
		offer(1, true, true, 5, 0, 1000), // no coordinates
		offer(2, true, true, 5, 80, 5000),
	}
	sortSupplierOffers(offers)
	if got := ids(offers); !equalIDs(got, 2, 1) {
		t.Fatalf("offers ordered %v, want [2 1]", got)
	}
}

// A comparator that contradicts itself lets sort return an arbitrary
// permutation, which is how in-stock offers used to end up below out-of-stock
// ones. Whatever the input order, every in-stock offer must precede every
// out-of-stock one.
func TestSortSupplierOffersIsAStrictWeakOrdering(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 200; trial++ {
		offers := make([]pages.SupplierOffer, 0, 12)
		for i := 0; i < 12; i++ {
			offers = append(offers, offer(
				int64(i+1),
				rng.Intn(2) == 0,
				rng.Intn(2) == 0,
				rng.Intn(3)*7,
				rng.Float64()*4, // sub-kilometre spreads: the old guard's blind spot
				int64(rng.Intn(5000)+100),
			))
		}
		sortSupplierOffers(offers)

		seenUnorderable := false
		for _, o := range offers {
			if !o.CanAddToCart {
				seenUnorderable = true
			} else if seenUnorderable {
				t.Fatalf("trial %d: an orderable offer sorted below an unorderable one", trial)
			}
		}
		// Within the orderable-and-covered tier, in stock comes first.
		seenOutOfStock := false
		for _, o := range offers {
			if !o.CanAddToCart || !o.IsCovered {
				continue
			}
			if o.AvailableStock == 0 {
				seenOutOfStock = true
			} else if seenOutOfStock {
				t.Fatalf("trial %d: an in-stock offer sorted below an out-of-stock one", trial)
			}
		}
	}
}

// Following a link to one supplier's listing surfaces that supplier — but never
// above an offer the pharmacy can order and this one cannot.
func TestPromoteFocusedOfferStaysInsideItsTier(t *testing.T) {
	offers := []pages.SupplierOffer{
		offer(10, true, true, 5, 1, 1000),
		offer(11, true, true, 5, 1, 1200),
		offer(12, false, false, 0, 1, 500),
	}

	promoteFocusedOffer(offers, 11)
	if got := ids(offers); !equalIDs(got, 11, 10, 12) {
		t.Fatalf("promoting a same-tier offer gave %v, want [11 10 12]", got)
	}

	promoteFocusedOffer(offers, 12)
	if got := ids(offers); !equalIDs(got, 11, 10, 12) {
		t.Fatalf("an unorderable offer was promoted over orderable ones: %v", got)
	}

	promoteFocusedOffer(offers, 999)
	if got := ids(offers); !equalIDs(got, 11, 10, 12) {
		t.Fatalf("an unknown variant reordered the list: %v", got)
	}
}
