package compare

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
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

// The board's total is the expensive half of the page — 38 ms of database CPU
// against 0.06–1.1 ms for the rows. It is read once per TTL, not once per view.
func TestMarketTotalIsCachedAcrossPageViews(t *testing.T) {
	svc := &Service{repo: &countingMarketRepo{total: 98370, rows: 24}}
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

// The rows are read first, and the page renders even when the total cannot be
// read at all.
//
// Before this rewrite the count came first and its error was the page's error,
// so a count that timed out on a cold cache took the whole board down with it:
// the reader got a 503 rather than twenty-four prices and a missing page number.
func TestMarketBoardRendersRowsWhenTheCountFails(t *testing.T) {
	svc := &Service{repo: &countingMarketRepo{rows: 24, countErr: errors.New("statement timeout")}}
	res, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1})
	if err != nil {
		t.Fatalf("a failed count must not fail the board: %v", err)
	}
	if len(res.Items) != 24 {
		t.Fatalf("got %d rows, want the 24 the query returned", len(res.Items))
	}
	if res.TotalKnown {
		t.Error("TotalKnown is true for a count that failed; the page would print a zero it invented")
	}
	if res.TotalPages != 0 {
		t.Errorf("TotalPages = %d, want 0 when the total is unknown", res.TotalPages)
	}
}

// Without a total, a full page still offers the next one. It is the only honest
// hint available and it is what keeps a degraded page navigable.
func TestMarketBoardOffersTheNextPageWithoutATotal(t *testing.T) {
	full := &Service{repo: &countingMarketRepo{rows: 24, countErr: errors.New("timeout")}}
	res, err := full.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasNext || !res.HasPrev {
		t.Errorf("a full page in the middle of the board should offer both ways: next=%v prev=%v", res.HasNext, res.HasPrev)
	}

	short := &Service{repo: &countingMarketRepo{rows: 6, countErr: errors.New("timeout")}}
	tail, err := short.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if tail.HasNext {
		t.Error("a page that came back short offered a next page")
	}
}

// A cached total must not outlive the board it describes. The stamp changes when
// a temporary warehouse is uploaded, archived or deleted; the key changes with
// it, so the next view recounts instead of serving yesterday's page count.
func TestMarketBoardCountIsInvalidatedWhenTheBoardChanges(t *testing.T) {
	repo := &stampingMarketRepo{countingMarketRepo: countingMarketRepo{total: 100, rows: 24}, stamp: "1:5:5"}
	svc := &Service{repo: repo}
	svc.SetCache(newFakeCache())

	if _, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1}); err != nil {
		t.Fatal(err)
	}
	if repo.counts != 1 {
		t.Fatalf("counted %d times while the board was unchanged, want 1", repo.counts)
	}

	// A warehouse was archived: the board holds fewer rows now.
	repo.stamp = "1:5:4"
	repo.total = 98
	res, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if repo.counts != 2 {
		t.Errorf("counted %d times after the board changed, want a recount", repo.counts)
	}
	if res.TotalCount != 98 {
		t.Errorf("total = %d, want the recounted 98", res.TotalCount)
	}
}

// The supplier dropdown is a convenience. Losing it must not cost the page.
func TestMarketBoardSurvivesASupplierListFailure(t *testing.T) {
	svc := &Service{repo: &countingMarketRepo{total: 10, rows: 3, supplierErr: errors.New("timeout")}}
	res, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1})
	if err != nil {
		t.Fatalf("a failed supplier list must not fail the board: %v", err)
	}
	if len(res.Items) != 3 || res.TotalCount != 10 {
		t.Errorf("rows or total lost with the dropdown: %+v", res)
	}
	if len(res.AvailableSuppliers) != 0 {
		t.Errorf("suppliers = %v, want none", res.AvailableSuppliers)
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
		"from date":    {DateFrom: "2026-10-29"},
		"to date":      {DateTo: "2026-10-29"},
		"query":        {Query: "بانادول"},
		"other query":  {Query: "كونجستال"},
		"supplier":     {Supplier: "مخزن المتحدة"},
		"min price":    {MinPrice: priceBound("10")},
		"max price":    {MaxPrice: priceBound("10")},
		"min discount": {MinDiscount: fp(10)},
		"max discount": {MaxDiscount: fp(10)},
		"zero bound":   {MinPrice: priceBound("0")},
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
	svc := &Service{repo: &countingMarketRepo{total: 42, rows: 24}}
	res, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalCount != 42 || !res.TotalKnown {
		t.Errorf("total = %d known=%v, want 42/true", res.TotalCount, res.TotalKnown)
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
	maxDiscount              float64
	rows                     int
	countErr, listErr        error
	supplierErr              error
	counts, lists, suppliers int
}

func (r *countingMarketRepo) ListMarketDiscounts(_ context.Context, f MarketDiscountsFilter) (*MarketDiscountsResult, error) {
	r.lists++
	if r.listErr != nil {
		return nil, r.listErr
	}
	limit := f.Limit
	if limit != 24 && limit != 48 && limit != 96 {
		limit = 24
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	items := make([]*MarketDiscountRow, 0, r.rows)
	for i := 0; i < r.rows; i++ {
		items = append(items, &MarketDiscountRow{ID: int64(i + 1)})
	}
	return &MarketDiscountsResult{
		Items:      items,
		Page:       page,
		Limit:      limit,
		TotalPages: 1,
	}, nil
}

func (r *countingMarketRepo) CountMarketDiscounts(context.Context, MarketDiscountsFilter) (MarketCount, error) {
	r.counts++
	if r.countErr != nil {
		return MarketCount{}, r.countErr
	}
	return MarketCount{Total: r.total, MaxDiscount: r.maxDiscount}, nil
}

func (r *countingMarketRepo) ListDistinctSuppliers(context.Context) ([]string, error) {
	r.suppliers++
	if r.supplierErr != nil {
		return nil, r.supplierErr
	}
	return []string{"مخزن المتحدة", "مخزن ابن سينا"}, nil
}

// stampingMarketRepo reports a board stamp, the way the Postgres repository does.
type stampingMarketRepo struct {
	countingMarketRepo
	stamp string
}

func (r *stampingMarketRepo) MarketBoardStamp(context.Context) (string, error) {
	return r.stamp, nil
}

func priceBound(value string) *money.Amount { amount := money.MustParse(value); return &amount }
