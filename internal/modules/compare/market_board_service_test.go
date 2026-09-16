package compare

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// fakeCache is an in-memory stand-in for Redis that records what it was asked
// for, so a test can tell a cache hit from a second trip to the database.
type fakeCache struct {
	items map[string][]byte
	sets  int
}

func newFakeCache() *fakeCache { return &fakeCache{items: map[string][]byte{}} }

func (c *fakeCache) GetJSON(_ context.Context, key string, dst any) error {
	raw, ok := c.items[key]
	if !ok {
		return errMiss
	}
	return json.Unmarshal(raw, dst)
}

func (c *fakeCache) SetJSON(_ context.Context, key string, val any, _ time.Duration) error {
	raw, err := json.Marshal(val)
	if err != nil {
		return err
	}
	c.items[key] = raw
	c.sets++
	return nil
}

type cacheMiss struct{}

func (cacheMiss) Error() string { return "miss" }

var errMiss = cacheMiss{}

// The board's total is the expensive half of the page — 33 ms of database CPU
// against 0.41 ms for the rows. It is read once per TTL, not once per view.
func TestMarketTotalIsCachedAcrossPageViews(t *testing.T) {
	svc := &Service{repo: &countingMarketRepo{total: 98370}}
	svc.SetCache(newFakeCache())
	repo := svc.repo.(*countingMarketRepo)

	for i := 0; i < 5; i++ {
		res, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1})
		if err != nil {
			t.Fatal(err)
		}
		if res.TotalCount != 98370 {
			t.Fatalf("total = %d, want 98370", res.TotalCount)
		}
	}

	if repo.counts != 1 {
		t.Errorf("counted %d times for 5 page views; the total should be cached", repo.counts)
	}
	if repo.suppliers != 1 {
		t.Errorf("listed suppliers %d times for 5 page views; it should be cached", repo.suppliers)
	}
	// The rows themselves are never cached: a price on this board is live.
	if repo.lists != 5 {
		t.Errorf("listed rows %d times for 5 page views; rows must never be cached", repo.lists)
	}
}

// Paging and sorting change which rows come back, never how many there are.
// Keying the cached total by them would multiply one number by every page of
// every ordering and turn a cache into a miss generator.
func TestMarketTotalCacheKeyIgnoresPagingAndSort(t *testing.T) {
	base := MarketDiscountsFilter{Query: "بانادول", Limit: 24, Page: 1}
	variants := []MarketDiscountsFilter{
		{Query: "بانادول", Limit: 96, Page: 7},
		{Query: "بانادول", SortBy: "price_desc"},
		{Query: "بانادول", SortBy: "oldest", Page: 3, Limit: 48},
	}
	want := marketFilterFingerprint(base)
	for _, v := range variants {
		if got := marketFilterFingerprint(v); got != want {
			t.Errorf("fingerprint changed with paging/sort: %+v", v)
		}
	}
}

// Anything that changes how many rows match must change the key, or one
// filter's total is served for another's.
func TestMarketTotalCacheKeySeparatesFilters(t *testing.T) {
	seen := map[string]string{}
	for name, f := range map[string]MarketDiscountsFilter{
		"empty":        {},
		"query":        {Query: "بانادول"},
		"other query":  {Query: "كونجستال"},
		"supplier":     {Supplier: "مخزن المتحدة"},
		"min price":    {MinPrice: fp(10)},
		"max price":    {MaxPrice: fp(10)},
		"min discount": {MinDiscount: fp(10)},
		"max discount": {MaxDiscount: fp(10)},
		"zero bound":   {MinPrice: fp(0)},
	} {
		key := marketFilterFingerprint(f)
		if prev, clash := seen[key]; clash {
			t.Errorf("%q and %q share a cache key; one filter's total would be served for the other", name, prev)
		}
		seen[key] = name
	}
}

// With no cache installed — tests, or Redis unreachable — the board must still
// render. Slow is a supported state; broken is not.
func TestMarketBoardWorksWithoutACache(t *testing.T) {
	svc := &Service{repo: &countingMarketRepo{total: 42}}
	res, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalCount != 42 {
		t.Errorf("total = %d, want 42", res.TotalCount)
	}
	if res.TotalPages != 2 {
		t.Errorf("total pages = %d, want 2 for 42 rows at 24 a page", res.TotalPages)
	}
	if !res.HasNext || res.HasPrev {
		t.Errorf("paging flags wrong on page 1 of 2: next=%v prev=%v", res.HasNext, res.HasPrev)
	}
}

func fp(v float64) *float64 { return &v }

// countingMarketRepo implements only the three methods the board reads and
// counts the calls. Embedding the interface leaves every other method nil, so a
// test that accidentally reaches one panics rather than passing quietly.
type countingMarketRepo struct {
	Repository
	total                    int64
	counts, lists, suppliers int
}

func (r *countingMarketRepo) ListMarketDiscounts(_ context.Context, f MarketDiscountsFilter) (*MarketDiscountsResult, error) {
	r.lists++
	limit := f.Limit
	if limit != 24 && limit != 48 && limit != 96 {
		limit = 24
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	return &MarketDiscountsResult{
		Items:      []*MarketDiscountRow{},
		Page:       page,
		Limit:      limit,
		TotalPages: 1,
	}, nil
}

func (r *countingMarketRepo) CountMarketDiscounts(context.Context, MarketDiscountsFilter) (int64, error) {
	r.counts++
	return r.total, nil
}

func (r *countingMarketRepo) ListDistinctSuppliers(context.Context) ([]string, error) {
	r.suppliers++
	return []string{"مخزن المتحدة", "مخزن ابن سينا"}, nil
}
