package compare

import (
	"context"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

type memoryCache struct {
	store map[string]any
}

func newMemoryCache() *memoryCache {
	return &memoryCache{store: make(map[string]any)}
}

func (m *memoryCache) GetJSON(ctx context.Context, key string, dst any) error {
	val, ok := m.store[key]
	if !ok {
		return apperrNotFound
	}
	switch d := dst.(type) {
	case *ComparisonResultSet:
		if v, ok := val.(*ComparisonResultSet); ok {
			*d = *v
			return nil
		}
	case *HeadToHeadComparisonResult:
		if v, ok := val.(*HeadToHeadComparisonResult); ok {
			*d = *v
			return nil
		}
	}
	return nil
}

var apperrNotFound = &cacheMissError{}

type cacheMissError struct{}

func (e *cacheMissError) Error() string { return "not found" }

func (m *memoryCache) SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error {
	m.store[key] = val
	return nil
}

func TestMultiSupplierCache_KeyDeterminismAndInvalidation(t *testing.T) {
	svc := &Service{}
	t1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 1, 12, 5, 0, 0, time.UTC)

	f1 := &CompareFile{ID: 1, UpdatedAt: t1}
	f2 := &CompareFile{ID: 2, UpdatedAt: t1}

	// Order independence: [f1, f2] must produce the same cache key as [f2, f1]
	key1 := svc.multiSupplierCacheKey([]*CompareFile{f1, f2})
	key2 := svc.multiSupplierCacheKey([]*CompareFile{f2, f1})
	if key1 == "" || key1 != key2 {
		t.Fatalf("expected identical cache keys regardless of order, got %q vs %q", key1, key2)
	}

	// Invalidation: when f1 is updated (new timestamp), key must change
	f1Updated := &CompareFile{ID: 1, UpdatedAt: t2}
	key3 := svc.multiSupplierCacheKey([]*CompareFile{f1Updated, f2})
	if key3 == key1 {
		t.Fatalf("expected different cache key after UpdatedAt changed, got identical key %q", key3)
	}
}

func TestHeadToHeadCache_KeyInvalidation(t *testing.T) {
	svc := &Service{}
	t1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 1, 12, 5, 0, 0, time.UTC)

	src := &CompareFile{ID: 10, UpdatedAt: t1}
	tgt := &CompareFile{ID: 20, UpdatedAt: t1}
	filter := HeadToHeadFilter{SourceFileID: 10, TargetFileID: 20, Query: "panadol"}

	key1 := svc.headToHeadCacheKey(src, tgt, filter)
	if key1 == "" {
		t.Fatal("expected non-empty cache key")
	}

	// Timestamp change on target file invalidates
	tgtUpdated := &CompareFile{ID: 20, UpdatedAt: t2}
	key2 := svc.headToHeadCacheKey(src, tgtUpdated, filter)
	if key1 == key2 {
		t.Fatalf("expected cache key to change when target UpdatedAt changes, got identical %q", key1)
	}

	// Filter change invalidates
	filter2 := filter
	filter2.Query = "congestal"
	key3 := svc.headToHeadCacheKey(src, tgt, filter2)
	if key1 == key3 {
		t.Fatalf("expected cache key to change when query changes, got identical %q", key1)
	}
}

func TestComparisonCache_GetAndSet(t *testing.T) {
	cache := newMemoryCache()
	svc := &Service{cache: cache}
	ctx := context.Background()

	t1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	files := []*CompareFile{{ID: 1, UpdatedAt: t1}}

	cached, key := svc.getCachedMultiSupplier(ctx, files)
	if cached != nil {
		t.Fatal("expected cache miss initially")
	}
	if key == "" {
		t.Fatal("expected non-empty key")
	}

	expectedResult := &ComparisonResultSet{
		Rows: []*ProductComparisonRow{
			{ProductName: "Panadol", BestPrice: money.MustParse("50.00")},
		},
		Summary: ComparisonSummary{TotalProducts: 1},
	}
	svc.setCachedMultiSupplier(ctx, key, expectedResult)

	hit, hitKey := svc.getCachedMultiSupplier(ctx, files)
	if hit == nil || len(hit.Rows) != 1 || hit.Rows[0].ProductName != "Panadol" {
		t.Fatalf("expected cache hit with Panadol, got %+v", hit)
	}
	if hitKey != key {
		t.Fatalf("expected key %q, got %q", key, hitKey)
	}
}
