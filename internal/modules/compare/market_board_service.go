package compare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Assembling one page of خصومات السوق العامة, cheaply.
//
// The board is public, unauthenticated, and the busiest read on the platform.
// It used to cost about 190 ms of database CPU per page view — three full
// passes over compare.file_rows for a page that shows twenty-four cards:
//
//	listing, with COUNT(*) OVER() ....... 145.7 ms
//	supplier filter dropdown ............  44.2 ms
//
// On the two vCPUs this platform's database runs on, that is roughly ten page
// views a second before the database is saturated and every other screen —
// orders, catalogue, the pharmacies' dashboards — slows down with it.
//
// The three passes are now one indexed read plus two numbers that barely
// change:
//
//	listing (idx_compare_file_rows_market_sort) ...... 0.41 ms, live
//	total count (33 ms uncached) ..................... cached, marketCountTTL
//	supplier names (0.97 ms uncached) ................ cached, marketCountTTL
//
// What is NOT cached is the only thing that must not be: the rows. A price on
// this board is always read live. The cache holds a row count and a list of
// warehouse names, both of which change only when a moderator uploads or
// archives a temporary warehouse — minutes to days apart, never mid-page.

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

// marketCountTTL bounds how stale the board's total and supplier list may be.
//
// Sixty seconds is chosen against what the numbers describe, not against how
// often the page is read. They move when a temporary warehouse is uploaded or
// archived; a minute later the page agrees. Nobody can perceive a pager that
// says 98,370 results when it has just become 98,394, and the rows themselves
// were never cached, so no price and no discount is ever a minute old.
const marketCountTTL = 60 * time.Second

// SetCache installs the cache used for the market board's totals.
func (s *Service) SetCache(c Cache) {
	s.cache = c
}

// ListMarketDiscounts returns one page of the public temporary-warehouse board.
func (s *Service) ListMarketDiscounts(ctx context.Context, filter MarketDiscountsFilter) (*MarketDiscountsResult, error) {
	result, err := s.repo.ListMarketDiscounts(ctx, filter)
	if err != nil {
		return nil, err
	}

	total, err := s.marketTotal(ctx, filter)
	if err != nil {
		return nil, err
	}
	result.TotalCount = total
	if total > 0 && result.Limit > 0 {
		result.TotalPages = int((total + int64(result.Limit) - 1) / int64(result.Limit))
	}
	result.HasPrev = result.Page > 1
	result.HasNext = result.Page < result.TotalPages

	suppliers, err := s.marketSuppliers(ctx)
	if err != nil {
		return nil, err
	}
	result.AvailableSuppliers = suppliers

	return result, nil
}

// marketTotal counts the rows behind the pager, through the cache.
func (s *Service) marketTotal(ctx context.Context, filter MarketDiscountsFilter) (int64, error) {
	key := "compare:market:count:" + marketFilterFingerprint(filter)
	var cached int64
	if s.cache != nil {
		if err := s.cache.GetJSON(ctx, key, &cached); err == nil {
			return cached, nil
		}
	}

	total, err := s.repo.CountMarketDiscounts(ctx, filter)
	if err != nil {
		return 0, err
	}
	if s.cache != nil {
		_ = s.cache.SetJSON(ctx, key, total, marketCountTTL)
	}
	return total, nil
}

// marketSuppliers lists the warehouses on the board, through the cache.
//
// One key, no filter: the dropdown offers every supplier the board holds, so
// that it can be used to narrow it. Narrowing it by supplier must not change
// which suppliers are offered.
func (s *Service) marketSuppliers(ctx context.Context) ([]string, error) {
	const key = "compare:market:suppliers"
	var cached []string
	if s.cache != nil {
		if err := s.cache.GetJSON(ctx, key, &cached); err == nil && len(cached) > 0 {
			return cached, nil
		}
	}

	suppliers, err := s.repo.ListDistinctSuppliers(ctx)
	if err != nil {
		return nil, err
	}
	if s.cache != nil && len(suppliers) > 0 {
		_ = s.cache.SetJSON(ctx, key, suppliers, marketCountTTL)
	}
	return suppliers, nil
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
	raw := fmt.Sprintf("q=%s|sup=%s|minp=%v|maxp=%v|mind=%v|maxd=%v",
		f.Query, f.Supplier,
		derefFloat(f.MinPrice), derefFloat(f.MaxPrice),
		derefFloat(f.MinDiscount), derefFloat(f.MaxDiscount))
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
