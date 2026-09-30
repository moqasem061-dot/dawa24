package matching_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestIndexCache_HitAndMiss(t *testing.T) {
	ctx := context.Background()
	var buildCount int64

	builder := matching.IndexBuilderFunc(func(ctx context.Context, scope string, revision int64, normVer string) (*productmatch.Index, error) {
		atomic.AddInt64(&buildCount, 1)
		prods := []productmatch.MasterProduct{
			{ID: revision, NameAR: "بندول 500 مجم"},
		}
		idx := productmatch.NewIndex(prods).WithCatalogRevision(revision)
		return idx, nil
	})

	cache := matching.NewIndexCache(builder)

	// 1. First get: Cache miss, builds rev 1
	idx1, err := cache.Get(ctx, "master", 1)
	if err != nil {
		t.Fatalf("cache.Get rev 1 failed: %v", err)
	}
	if atomic.LoadInt64(&buildCount) != 1 {
		t.Errorf("buildCount = %d, want 1", atomic.LoadInt64(&buildCount))
	}
	rev, _ := idx1.Revision()
	if rev != 1 {
		t.Errorf("idx1 rev = %d, want 1", rev)
	}

	// 2. Second get: Cache hit, does not build
	idx1Hit, err := cache.Get(ctx, "master", 1)
	if err != nil {
		t.Fatalf("cache.Get rev 1 hit failed: %v", err)
	}
	if idx1Hit != idx1 {
		t.Errorf("expected same index pointer on hit")
	}
	if atomic.LoadInt64(&buildCount) != 1 {
		t.Errorf("buildCount after hit = %d, want 1", atomic.LoadInt64(&buildCount))
	}

	// 3. Revision bump: Cache miss, builds rev 2
	idx2, err := cache.Get(ctx, "master", 2)
	if err != nil {
		t.Fatalf("cache.Get rev 2 failed: %v", err)
	}
	if idx2 == idx1 {
		t.Errorf("expected different index pointer on revision bump")
	}
	if atomic.LoadInt64(&buildCount) != 2 {
		t.Errorf("buildCount after rev 2 = %d, want 2", atomic.LoadInt64(&buildCount))
	}
	if cache.ScopeLen("master") != 2 {
		t.Errorf("scope len = %d, want 2", cache.ScopeLen("master"))
	}

	// 4. Revision bump to 3: Cache miss, builds rev 3, evicts rev 1 (max 2 entries per scope)
	idx3, err := cache.Get(ctx, "master", 3)
	if err != nil {
		t.Fatalf("cache.Get rev 3 failed: %v", err)
	}
	_ = idx3
	if atomic.LoadInt64(&buildCount) != 3 {
		t.Errorf("buildCount after rev 3 = %d, want 3", atomic.LoadInt64(&buildCount))
	}
	if cache.ScopeLen("master") != 2 {
		t.Errorf("scope len after eviction = %d, want 2", cache.ScopeLen("master"))
	}

	// 5. Calling rev 1 now triggers a new build because it was evicted
	_, err = cache.Get(ctx, "master", 1)
	if err != nil {
		t.Fatalf("cache.Get evicted rev 1 failed: %v", err)
	}
	if atomic.LoadInt64(&buildCount) != 4 {
		t.Errorf("buildCount after evicted rev 1 = %d, want 4", atomic.LoadInt64(&buildCount))
	}
}

func TestIndexCache_Singleflight_Concurrent20(t *testing.T) {
	ctx := context.Background()
	var buildCount int64

	builder := matching.IndexBuilderFunc(func(ctx context.Context, scope string, revision int64, normVer string) (*productmatch.Index, error) {
		atomic.AddInt64(&buildCount, 1)
		// Simulate slight build delay
		time.Sleep(20 * time.Millisecond)
		prods := []productmatch.MasterProduct{
			{ID: 100, NameAR: "كونكور 5 مجم"},
		}
		idx := productmatch.NewIndex(prods).WithCatalogRevision(revision)
		return idx, nil
	})

	cache := matching.NewIndexCache(builder)

	const callers = 20
	var wg sync.WaitGroup
	indices := make([]*productmatch.Index, callers)
	errors := make([]error, callers)

	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(idx int) {
			defer wg.Done()
			indices[idx], errors[idx] = cache.Get(ctx, "concurrent_test", 100)
		}(i)
	}
	wg.Wait()

	for i := 0; i < callers; i++ {
		if errors[i] != nil {
			t.Fatalf("caller %d failed: %v", i, errors[i])
		}
		if indices[i] == nil {
			t.Fatalf("caller %d got nil index", i)
		}
		if indices[i] != indices[0] {
			t.Errorf("caller %d got different index pointer than caller 0", i)
		}
	}

	if count := atomic.LoadInt64(&buildCount); count != 1 {
		t.Errorf("buildCount under %d concurrent callers = %d; want exactly 1", callers, count)
	}
}
