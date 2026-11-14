package compare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Assembling one page of خصومات السوق العامة.
//
// This is the second rewrite of this path, and the first one that starts from
// the rows.
//
// What the page cost before (100,000 live rows, two vCPUs, measured):
//
//	default ordering (newest) ....... 118.6 ms   seq scan + hash join + sort
//	highest discount ................ 108.1 ms   sort (its index had stopped
//	                                             matching the expression)
//	the pager's total ...............  38.0 ms   COUNT(*) over every row
//	supplier dropdown ...............   0.6 ms
//
// A cold view paid ~160 ms of database CPU to render twenty-four cards, and the
// page did it in the request path: first the total, then — only if that total
// was non-zero — the rows. Every one of those steps could fail the page, and
// the first one was the slowest.
//
// After the rewrite (db/migrations/246 + this file):
//
//	rows, any sort, page 1 ..........  0.06–1.1 ms  index scan, LIMIT stops it
//	rows, page 500 ..................  3.1 ms       index scan, OFFSET walks it
//	total ...........................  cached, 3 minutes, keyed by the board
//	                                  stamp below, bounded by marketCountBudget
//	supplier dropdown ...............  cached, 1 minute
//
// Three decisions carry that:
//
//   - **Rows first.** The list is the page; the total is a label on it. Reading
//     the count first meant the cheapest, most cacheable part of the query
//     gated the part that had to be live.
//
//   - **A bounded total.** The count runs against a deadline. On a miss that
//     overruns it, the board renders with its rows and without a page count,
//     which is a worse label and a working screen — instead of a timeout page.
//
//   - **A board stamp in the cache key.** The cached total used to expire on a
//     timer alone, so it could be three minutes stale and had no way to know a
//     warehouse had been uploaded or archived. The stamp is one cheap aggregate
//     over compare.files (a few hundred rows); when it changes, every cached
//     count for the board misses at once. Prices are still never cached: the
//     stamp describes which rows exist, not what they cost.

// Cache is the subset of the platform cache this module needs.
//
// Declared here rather than imported so the module does not depend on Redis:
// it is nil in tests and whenever the cache is unreachable, and every path
// below falls through to the database when it is. Redis being down makes this
// board slow, not broken.
type Cache interface {
	GetJSON(ctx context.Context, key string, dst any) error
	SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error
}

const (
	// marketCountTTL bounds how stale the board's total and supplier list may
	// be. The board stamp invalidates them the moment a warehouse is uploaded,
	// archived or deleted; the TTL is what bounds the damage when the stamp
	// itself cannot be read.
	marketCountTTL = 3 * time.Minute

	// marketSuppliersTTL is longer than the count's because the dropdown is a
	// list of names that only changes with an upload, and a stale name in a
	// suggestion list costs nothing.
	marketSuppliersTTL = 5 * time.Minute

	// marketCountBudget is how long the page will wait for an exact total.
	//
	// The count is 38 ms on 100,000 rows uncached and is normally a cache hit
	// in under a millisecond. This deadline exists for the case where it is
	// neither: a cold cache, a busy database, a pathological filter. Beyond it
	// the reader gets the page without the pager's page numbers, which is a
	// label missing — not a screen missing.
	marketCountBudget = 1200 * time.Millisecond
)

// SetCache installs the cache used for the market board's totals.
func (s *Service) SetCache(c Cache) {
	s.cache = c
}

// marketStamper is implemented by the Postgres repository, and only by it.
//
// Declared as an optional interface rather than added to Repository so that the
// dozen test doubles for the module keep compiling: a double that does not
// implement it simply never invalidates a count early, which is exactly the
// state the TTL covers.
type marketStamper interface {
	MarketBoardStamp(ctx context.Context) (string, error)
}

// ListMarketDiscounts returns one page of the public temporary-warehouse board.
func (s *Service) ListMarketDiscounts(ctx context.Context, filter MarketDiscountsFilter) (*MarketDiscountsResult, error) {
	filter.Query = marketQuery(filter.Query)
	filter.CatalogIDs = nil

	result, err := s.repo.ListMarketDiscounts(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Nothing matched the indexed text search. The reader may have typed the name
	// the way a supplier's sheet spells it rather than the way the catalogue
	// does, so ask the catalogue for candidates and try once more. This is only
	// worth a second query when the first found nothing — which is why it is
	// decided from the rows now, and used to be decided from the count.
	if len(result.Items) == 0 && filter.Query != "" {
		if ids := s.marketCatalogIDs(ctx, filter.Query); len(ids) > 0 {
			filter.CatalogIDs = ids
			retried, retryErr := s.repo.ListMarketDiscounts(ctx, filter)
			if retryErr != nil {
				s.warn(ctx, "market board: catalogue candidate retry failed", retryErr)
			} else {
				result = retried
			}
		}
	}

	s.decorateBoard(ctx, filter, result)
	s.nameBoardProduct(ctx, filter, result)
	return result, nil
}

// marketProductNamer is implemented by the Postgres repository, and only by it,
// for the same reason as marketStamper: a double that does not implement it just
// leaves the product chip without a name.
type marketProductNamer interface {
	MarketProductNames(ctx context.Context, productID int64) (ar, en string, err error)
}

// nameBoardProduct fills the name of the product the board is filtered to, so
// the page can say "الصنف: <name>" even on a page with no rows. A failed lookup
// leaves the name empty; the filter itself is unaffected.
func (s *Service) nameBoardProduct(ctx context.Context, filter MarketDiscountsFilter, result *MarketDiscountsResult) {
	if filter.ProductID == nil {
		return
	}
	for _, item := range result.Items {
		if item.CatalogNameAR != "" || item.CatalogNameEN != "" {
			result.ProductNameAR, result.ProductNameEN = item.CatalogNameAR, item.CatalogNameEN
			return
		}
	}
	namer, ok := s.repo.(marketProductNamer)
	if !ok {
		return
	}
	ar, en, err := namer.MarketProductNames(ctx, *filter.ProductID)
	if err != nil {
		s.warn(ctx, "market board: product name unavailable", err)
		return
	}
	result.ProductNameAR, result.ProductNameEN = ar, en
}

// decorateBoard adds what the page needs around the rows: the pager, and the
// warehouse list for the filter. Neither can fail the page — they are read,
// logged, and left out if they are unavailable.
func (s *Service) decorateBoard(ctx context.Context, filter MarketDiscountsFilter, result *MarketDiscountsResult) {
	count, known := s.marketTotal(ctx, filter)
	total := count.Total
	result.TotalCount = total
	result.MaxDiscount = count.MaxDiscount
	result.TotalKnown = known
	if known {
		if result.Limit > 0 {
			result.TotalPages = int((total + int64(result.Limit) - 1) / int64(result.Limit))
		}
	} else {
		result.TotalPages = 0
	}
	result.HasPrev = result.Page > 1
	if known {
		result.HasNext = result.Page < result.TotalPages
	} else {
		// No exact total, so no way to know where the last page is. A full page
		// is the only honest hint available, and it is only ever used to draw a
		// "next page" link.
		result.HasNext = len(result.Items) >= result.Limit
	}

	suppliers, err := s.marketSuppliers(ctx)
	if err != nil {
		s.warn(ctx, "market board: supplier list unavailable", err)
		return
	}
	result.AvailableSuppliers = suppliers
}

// marketTotal counts the rows behind the pager and finds the board's highest
// discount, in one scan, through the cache, under a deadline. The bool reports
// whether the numbers are known; false is a page without a pager and without a
// highest discount, not an error.
//
// The maximum shares the count's key and its budget on purpose: it is the same
// filter over the same rows, so it is exactly as stale as the count and costs
// no second round trip.
func (s *Service) marketTotal(ctx context.Context, filter MarketDiscountsFilter) (MarketCount, bool) {
	key := ""
	if s.cache != nil {
		key = "compare:market:count:v6:" + s.boardStamp(ctx) + ":" + marketFilterFingerprint(filter)
		var cached MarketCount
		if err := s.cache.GetJSON(ctx, key, &cached); err == nil { // nolint:errswallow: a cache miss or error recomputes the count below
			return cached, true
		}
	}

	countCtx, cancel := context.WithTimeout(ctx, marketCountBudget)
	defer cancel()
	count, err := s.repo.CountMarketDiscounts(countCtx, filter)
	if err != nil {
		s.warn(ctx, "market board: count unavailable, rendering without a pager", err)
		return MarketCount{}, false
	}
	if s.cache != nil {
		if err := s.cache.SetJSON(ctx, key, count, marketCountTTL); err != nil {
			s.warn(ctx, "market board: could not cache the count", err)
		}
	}
	return count, true
}

// boardStamp names the board's current contents.
//
// It changes when a temporary warehouse is added, removed, archived or restored
// — every event that changes how many rows the board holds. It deliberately
// does not change when a price does: prices are read live, and invalidating the
// count for a price edit would only cost a recount that returns the same number.
//
// An unreadable stamp returns a constant, which keeps the cache working and
// leaves the TTL in charge of staleness.
func (s *Service) boardStamp(ctx context.Context) string {
	stamper, ok := s.repo.(marketStamper)
	if !ok {
		return "static"
	}
	stamp, err := stamper.MarketBoardStamp(ctx)
	if err != nil {
		s.warn(ctx, "market board: board stamp unavailable", err)
		return "unknown"
	}
	return stamp
}

// marketSuppliers lists the warehouses on the board, through the cache.
//
// One key, no filter: the dropdown offers every supplier the board holds, so
// that it can be used to narrow it. Narrowing it by supplier must not change
// which suppliers are offered.
func (s *Service) marketSuppliers(ctx context.Context) ([]string, error) {
	const key = "compare:market:suppliers:v2"
	var cached []string
	if s.cache != nil {
		if err := s.cache.GetJSON(ctx, key, &cached); err == nil && len(cached) > 0 { // nolint:errswallow: a cache miss or error recomputes the list below
			return cached, nil
		}
	}

	suppliers, err := s.repo.ListDistinctSuppliers(ctx)
	if err != nil {
		return nil, err
	}
	if s.cache != nil && len(suppliers) > 0 {
		if err := s.cache.SetJSON(ctx, key, suppliers, marketSuppliersTTL); err != nil {
			s.warn(ctx, "market board: could not cache the supplier list", err)
		}
	}
	return suppliers, nil
}

func (s *Service) warn(ctx context.Context, msg string, err error) {
	if s.log != nil {
		s.log.WarnContext(ctx, msg, "error", err)
	}
}

// marketFilterFingerprint names one filter's count.
//
// Only the fields the COUNT depends on go in: page, limit and sort order change
// which rows come back but never how many there are, and including them would
// multiply one cached number by every page and every ordering of the board.
//
// Hashed rather than concatenated because Query and Supplier are free text
// arriving from a URL, and a raw one would put user input — colons, spaces,
// newlines — into a Redis key namespace that uses ':' as its separator.
func marketFilterFingerprint(f MarketDiscountsFilter) string {
	raw := fmt.Sprintf("from=%s|to=%s|q=%s|sup=%s|minp=%v|maxp=%v|mind=%v|maxd=%v|ids=%v|pid=%s",
		f.DateFrom, f.DateTo, f.Query, f.Supplier,
		marketPriceBound(f.MinPrice), marketPriceBound(f.MaxPrice),
		derefFloat(f.MinDiscount), derefFloat(f.MaxDiscount), f.CatalogIDs, derefInt(f.ProductID))
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:12])
}

// derefFloat renders an optional bound so that "absent" and "zero" are
// different fingerprints.
func derefFloat(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%g", *v)
}

func derefInt(v *int64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *v)
}

func marketPriceBound(value *money.Amount) string {
	if value == nil {
		return "-"
	}
	return value.String()
}
