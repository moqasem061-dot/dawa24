package matching

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// IndexCacheKey identifies a specific catalogue index instance in the cache.
type IndexCacheKey struct {
	Scope             string
	Revision          int64
	NormalizerVersion string
}

func (k IndexCacheKey) String() string {
	return fmt.Sprintf("%s:%d:%s", k.Scope, k.Revision, k.NormalizerVersion)
}

type cacheEntry struct {
	index    *productmatch.Index
	accessed time.Time
}

// IndexBuilder loads catalogue products and constructs an Index.
type IndexBuilder interface {
	BuildIndex(ctx context.Context, scope string, revision int64, normalizerVer string) (*productmatch.Index, error)
}

// IndexBuilderFunc is an adapter to allow the use of ordinary functions as IndexBuilder.
type IndexBuilderFunc func(ctx context.Context, scope string, revision int64, normalizerVer string) (*productmatch.Index, error)

// BuildIndex calls f(ctx, scope, revision, normalizerVer).
func (f IndexBuilderFunc) BuildIndex(ctx context.Context, scope string, revision int64, normalizerVer string) (*productmatch.Index, error) {
	return f(ctx, scope, revision, normalizerVer)
}

// IndexCache is a thread-safe LRU cache for *productmatch.Index instances,
// holding at most 2 entries per scope and collapsing concurrent builds with singleflight.
type IndexCache struct {
	mu      sync.RWMutex
	entries map[IndexCacheKey]*cacheEntry
	scopes  map[string][]IndexCacheKey
	group   singleflight.Group
	builder IndexBuilder
}

// NewIndexCache creates a new IndexCache with the given builder.
func NewIndexCache(builder IndexBuilder) *IndexCache {
	return &IndexCache{
		entries: make(map[IndexCacheKey]*cacheEntry),
		scopes:  make(map[string][]IndexCacheKey),
		builder: builder,
	}
}

// NewIndexCacheWithCuration creates an IndexCache backed by a Loader and CuratedRepository.
func NewIndexCacheWithCuration(loader *Loader, curatedRepo CuratedRepository) *IndexCache {
	return NewIndexCache(NewDefaultIndexBuilderWithCuration(loader, curatedRepo))
}

// Get retrieves an existing index or builds it through the builder, collapsing concurrent requests.
func (c *IndexCache) Get(ctx context.Context, scope string, revision int64) (*productmatch.Index, error) {
	if scope == "" {
		scope = "master"
	}
	normVer := productmatch.NormalizerVersion
	key := IndexCacheKey{
		Scope:             scope,
		Revision:          revision,
		NormalizerVersion: normVer,
	}

	// 1. Fast read lock check
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		entry.accessed = time.Now()
		c.touchScopeKeyLocked(scope, key)
		c.mu.Unlock()
		return entry.index, nil
	}
	c.mu.Unlock()

	// 2. Singleflight build
	keyStr := key.String()
	res, err, _ := c.group.Do(keyStr, func() (any, error) {
		// Double-check with lock in case another singleflight group just populated it
		c.mu.Lock()
		if entry, ok := c.entries[key]; ok {
			entry.accessed = time.Now()
			c.touchScopeKeyLocked(scope, key)
			c.mu.Unlock()
			return entry.index, nil
		}
		c.mu.Unlock()

		if c.builder == nil {
			return nil, fmt.Errorf("no index builder configured")
		}

		idx, err := c.builder.BuildIndex(ctx, scope, revision, normVer)
		if err != nil {
			return nil, err
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		c.putLocked(key, idx)
		return idx, nil
	})

	if err != nil {
		return nil, err
	}
	return res.(*productmatch.Index), nil
}

func (c *IndexCache) putLocked(key IndexCacheKey, idx *productmatch.Index) {
	scope := key.Scope
	keys := c.scopes[scope]

	if _, ok := c.entries[key]; ok {
		c.entries[key] = &cacheEntry{index: idx, accessed: time.Now()}
		c.touchScopeKeyLocked(scope, key)
		return
	}

	// If scope has >= 2 entries, evict the oldest
	for len(keys) >= 2 {
		oldest := keys[0]
		keys = keys[1:]
		delete(c.entries, oldest)
	}

	keys = append(keys, key)
	c.scopes[scope] = keys
	c.entries[key] = &cacheEntry{index: idx, accessed: time.Now()}
}

func (c *IndexCache) touchScopeKeyLocked(scope string, key IndexCacheKey) {
	keys := c.scopes[scope]
	idx := -1
	for i, k := range keys {
		if k == key {
			idx = i
			break
		}
	}
	if idx >= 0 {
		keys = append(keys[:idx], keys[idx+1:]...)
		keys = append(keys, key)
		c.scopes[scope] = keys
	}
}

// Len returns the total number of cached index entries across all scopes.
func (c *IndexCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// ScopeLen returns the number of cached index entries for a given scope.
func (c *IndexCache) ScopeLen(scope string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.scopes[scope])
}
