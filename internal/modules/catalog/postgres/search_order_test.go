package postgres

import (
	"strings"
	"testing"
)

// Availability leads when browsing and breaks ties when searching.
//
// The catalogue paginates in SQL at the product level; the Go layer's re-sort
// only reorders the twenty-four rows it was handed, which does nothing when the
// orderable products are spread across the whole catalogue. Putting
// availability into the SQL ordering is what makes page one buyable — but doing
// it unconditionally would rank an irrelevant in-stock product above the
// medicine someone typed the name of.
func TestSearchOrderPutsAvailabilityFirstOnlyWhenBrowsing(t *testing.T) {
	t.Run("no query: availability leads", func(t *testing.T) {
		if got := searchOrderPrefix("", ""); !strings.Contains(got, "EXISTS") {
			t.Errorf("browsing must order by availability first, got %q", got)
		}
		if got := searchOrderSuffix(""); got != "" {
			t.Errorf("availability must not also appear after relevance, got %q", got)
		}
	})

	t.Run("with query: relevance leads", func(t *testing.T) {
		if got := searchOrderPrefix("بانادول", ""); got != "" {
			t.Errorf("a search must order by relevance first, got %q", got)
		}
		if got := searchOrderSuffix("بانادول"); !strings.Contains(got, "EXISTS") {
			t.Errorf("availability must break relevance ties, got %q", got)
		}
	})

	t.Run("whitespace is not a query", func(t *testing.T) {
		if got := searchOrderPrefix("   ", ""); !strings.Contains(got, "EXISTS") {
			t.Errorf("a blank query is browsing, got %q", got)
		}
	})
}

// The ordering and the in-stock filter must ask the same question.
//
// They were separate copies of the same EXISTS, and a change to one would have
// silently produced a first page ordered by a rule the filter did not share.
func TestStockPredicateIsSharedBySortAndFilter(t *testing.T) {
	if !strings.Contains(stockFirstOrder, productHasStockSQL) {
		t.Error("the ordering does not use the shared availability predicate")
	}
	if !strings.Contains(productHasStockSQL, "status = 'active'") {
		t.Error("the availability predicate must filter on active variant status")
	}
}
