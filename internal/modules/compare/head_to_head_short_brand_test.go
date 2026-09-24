package compare

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// A short skeleton that names two rows on one side is ambiguous, and an
// unmatched row is better than a price compared against the wrong medicine.
func TestShortBrandPairsRefusesAnAmbiguousKey(t *testing.T) {
	p := money.FromMajor(45)
	src := []*CompareFileRow{{RawName: "Mobic 15mg", Price: p}}
	one := []*CompareFileRow{{RawName: "موبيك 15 مجم", Price: p}}
	two := []*CompareFileRow{{RawName: "موبيك 15 مجم", Price: p}, {RawName: "موبك 15 مجم", Price: p}}

	if newShortBrandPairs(src, one).pair(src[0]) == nil {
		t.Fatal("an unambiguous short brand did not pair")
	}
	if got := newShortBrandPairs(src, two).pair(src[0]); got != nil {
		t.Errorf("an ambiguous short brand paired with %q", got.RawName)
	}
}
