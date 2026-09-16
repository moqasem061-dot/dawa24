package cache

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// DefaultL1Capacity is the maximum number of items held in memory per cache instance.
const DefaultL1Capacity = 5000

type l1Item[T any] struct {
	val       T
	expiresAt time.Time
}

// L1 is a concurrency-safe, in-process micro-cache with TTL and stampede protection.
// It sits in front of Redis and PostgreSQL to serve hot reference data in < 5 µs.
type L1[T any] struct {
	mu       sync.RWMutex
	items    map[string]l1Item[T]
	group    singleflight.Group
	capacity int
}

// NewL1 creates an in-memory micro-cache with the default capacity.
func NewL1[T any]() *L1[T] {
	return NewL1WithCapacity[T](DefaultL1Capacity)
}

// NewL1WithCapacity creates an in-memory micro-cache with a custom item capacity.
func NewL1WithCapacity[T any](capacity int) *L1[T] {
	if capacity <= 0 {
		capacity = DefaultL1Capacity
	}
	return &L1[T]{
		items:    make(map[string]l1Item[T], 64),
		capacity: capacity,
	}
}

// Get retrieves an item from the cache. Returns false if missing or expired.
func (c *L1[T]) Get(key string) (T, bool) {
	now := time.Now()
	c.mu.RLock()
	item, found := c.items[key]
	c.mu.RUnlock()

	if !found || now.After(item.expiresAt) {
		var zero T
		return zero, false
	}
	return item.val, true
}

// Set stores an item in the cache with the specified TTL.
func (c *L1[T]) Set(key string, val T, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.items) >= c.capacity {
		// Prune expired entries to maintain memory ceiling
		for k, it := range c.items {
			if now.After(it.expiresAt) {
				delete(c.items, k)
			}
		}
		// If still at capacity, wholesale wipe half the entries to avoid memory growth
		if len(c.items) >= c.capacity {
			count := 0
			for k := range c.items {
				delete(c.items, k)
				count++
				if count >= c.capacity/2 {
					break
				}
			}
		}
	}

	c.items[key] = l1Item[T]{
		val:       val,
		expiresAt: now.Add(ttl),
	}
}

// Remember returns the cached item or runs compute under singleflight synchronization.
func (c *L1[T]) Remember(key string, ttl time.Duration, compute func() (T, error)) (T, error) {
	if val, ok := c.Get(key); ok {
		return val, nil
	}

	v, err, _ := c.group.Do(key, func() (any, error) {
		// Double-check under singleflight barrier
		if val, ok := c.Get(key); ok {
			return val, nil
		}
		computed, err := compute()
		if err != nil {
			return nil, err
		}
		c.Set(key, computed, ttl)
		return computed, nil
	})

	if err != nil {
		var zero T
		return zero, err
	}
	res, ok := v.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("l1: unexpected type %T", v)
	}
	return res, nil
}

// Invalidate removes a specific key.
func (c *L1[T]) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// InvalidatePrefix removes all keys matching the prefix.
func (c *L1[T]) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.items {
		if strings.HasPrefix(k, prefix) {
			delete(c.items, k)
		}
	}
}

// InvalidateAll removes all entries from the cache.
func (c *L1[T]) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]l1Item[T], 64)
}

// Len returns the current number of cached entries.
func (c *L1[T]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}
