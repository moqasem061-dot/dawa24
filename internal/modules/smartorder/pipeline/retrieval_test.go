package pipeline

import (
	"context"
	"fmt"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// The query-count gate.
//
// FR-017a exists because the previous generation of this feature issued a query
// per row: a ten-thousand-line import spent its entire budget waiting on round
// trips. That regression is invisible in a unit test that only checks results —
// the answers are right, they just take an hour — so it is asserted directly.
//
// A single accidental lookup inside a loop is all it takes, and it is the kind
// of change that looks harmless in review.

// countingRepo records how many times each bulk method is called.
type countingRepo struct {
	smartorder.Repository
	codes     int
	saving    int
	learned   int
	exactName int
	fuzzyDB   int
	contains  int
	alias     int
	offers    int
	index     int
}

func (c *countingRepo) ResolveByCodes(context.Context, []string, []string) (map[string]int64, error) {
	c.codes++
	return map[string]int64{}, nil
}

func (c *countingRepo) ResolveBySaving(context.Context, int64, []string, []string) (map[string]int64, error) {
	c.saving++
	return map[string]int64{}, nil
}

func (c *countingRepo) ResolveByLearned(context.Context, int64, []string) (map[string]int64, error) {
	c.learned++
	return map[string]int64{}, nil
}

func (c *countingRepo) ResolveByExactName(context.Context, []string, string) (map[string]int64, error) {
	c.exactName++
	return map[string]int64{}, nil
}

func (c *countingRepo) ResolveByFuzzyDB(context.Context, []string, string) (map[string]int64, error) {
	c.fuzzyDB++
	return map[string]int64{}, nil
}

func (c *countingRepo) ResolveByContains(context.Context, []string, string) (map[string]int64, error) {
	c.contains++
	return map[string]int64{}, nil
}

func (c *countingRepo) ResolveByAlias(context.Context, []string) (map[string]int64, error) {
	c.alias++
	return map[string]int64{}, nil
}

func (c *countingRepo) LoadOffers(context.Context, int64, []int64) ([]smartorder.Offer, error) {
	c.offers++
	return nil, nil
}

func (c *countingRepo) LoadMatchIndex(context.Context) ([]smartorder.IndexedProduct, error) {
	c.index++
	return nil, nil
}

func (c *countingRepo) total() int {
	return c.codes + c.saving + c.learned + c.exactName + c.fuzzyDB + c.contains + c.alias + c.offers + c.index
}

func linesFor(n int) []*smartorder.Line {
	out := make([]*smartorder.Line, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, &smartorder.Line{
			ID:        int64(i),
			RowNumber: i,
			RawName:   fmt.Sprintf("منتج رقم %d", i),
			RawSKU:    fmt.Sprintf("SKU-%d", i),
			Outcome:   smartorder.OutcomeUnmatched,
		})
	}
	return out
}

// 🚦 The gate: resolution cost must not scale with row count.
func TestResolveQueryCountIsIndependentOfRowCount(t *testing.T) {
	cfg := &smartorder.Config{OrganizationID: 50, UseSavingProducts: true}

	var counts []int
	for _, size := range []int{10, 100, 5000} {
		repo := &countingRepo{}
		lines := linesFor(size)
		Normalize(lines)

		if err := NewResolver(repo, cfg).Resolve(context.Background(), lines); err != nil {
			t.Fatalf("%d rows: %v", size, err)
		}
		counts = append(counts, repo.total())
	}

	for i, got := range counts {
		if got != counts[0] {
			t.Fatalf("query count changed with row count: %v — a per-row lookup has crept in (case %d)", counts, i)
		}
	}
	// Five tiers, five queries, whatever the file size.
	if counts[0] > 7 {
		t.Fatalf("expected a handful of queries for the whole file, got %d", counts[0])
	}
}

// Disabling Saving Products must remove its query entirely, not merely ignore
// the result: FR-015 requires the links be consulted by no tier at all.
func TestSavingProductsQueryIsSkippedWhenToggleIsOff(t *testing.T) {
	lines := linesFor(50)
	Normalize(lines)

	repo := &countingRepo{}
	cfg := &smartorder.Config{OrganizationID: 50, UseSavingProducts: false}
	if err := NewResolver(repo, cfg).Resolve(context.Background(), lines); err != nil {
		t.Fatal(err)
	}
	if repo.saving != 0 {
		t.Fatalf("saving products must not be queried when the toggle is off, got %d calls", repo.saving)
	}

	repo2 := &countingRepo{}
	cfg2 := &smartorder.Config{OrganizationID: 50, UseSavingProducts: true}
	lines2 := linesFor(50)
	Normalize(lines2)
	if err := NewResolver(repo2, cfg2).Resolve(context.Background(), lines2); err != nil {
		t.Fatal(err)
	}
	if repo2.saving != 1 {
		t.Fatalf("expected exactly one saving-products query for the whole file, got %d", repo2.saving)
	}
}

// Lookup keys are deduplicated: a file listing the same product a hundred times
// must not send it a hundred times.
func TestUnresolvedKeysAreDeduplicated(t *testing.T) {
	lines := make([]*smartorder.Line, 0, 100)
	for i := 0; i < 100; i++ {
		lines = append(lines, &smartorder.Line{ID: int64(i + 1), RawName: "بانادول", RawSKU: "P1"})
	}
	Normalize(lines)

	names, skus := unresolvedKeys(lines)
	if len(names) != 1 {
		t.Fatalf("expected one distinct name, got %d", len(names))
	}
	if len(skus) != 1 {
		t.Fatalf("expected one distinct sku, got %d", len(skus))
	}
}

// Resolved lines drop out, so each tier sees a smaller set than the last.
func TestResolvedLinesAreNotReQueried(t *testing.T) {
	lines := linesFor(10)
	Normalize(lines)
	productID := int64(99)
	for i := 0; i < 5; i++ {
		lines[i].MatchedProductID = &productID
		lines[i].MatchConfidence = 1.0
	}

	names, _ := unresolvedKeys(lines)
	if len(names) != 5 {
		t.Fatalf("already-matched lines must not be looked up again, got %d keys", len(names))
	}
}

// A line at or above the cutoff is off-limits to AI (FR-018).
func TestUnresolvedRespectsTheCutoff(t *testing.T) {
	productID := int64(7)
	lines := []*smartorder.Line{
		{ID: 1, MatchedProductID: &productID, MatchConfidence: Cutoff},
		{ID: 2, MatchedProductID: &productID, MatchConfidence: Cutoff - 0.001},
		{ID: 3},
	}
	residual := Unresolved(lines)
	if len(residual) != 2 {
		t.Fatalf("expected the below-cutoff and unmatched lines only, got %d", len(residual))
	}
	for _, r := range residual {
		if r.ID == 1 {
			t.Fatal("a line at the cutoff is resolved and must never be sent for adjudication")
		}
	}
}
