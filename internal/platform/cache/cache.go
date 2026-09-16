// Package cache wraps Redis for caching, sessions and rate limiting.
//
// The legacy system ran all three on MySQL (CACHE_STORE=database,
// SESSION_DRIVER=database, plus a jobs table), so every page view wrote to the
// same database handling order inserts. Moving them to Redis removes that write
// amplification from the primary.
//
// The tenant rule in this package is absolute: every cache key is namespaced by
// organisation. A key without a tenant prefix is a cross-tenant data leak that
// no amount of row-level security will catch, because the data never reaches the
// database on a cache hit.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

// ErrMiss signals a key that is absent. Callers treat this as "compute it",
// never as a failure.
var ErrMiss = errors.New("cache: miss")

// ErrNotConnected is returned before the client has been established.
var ErrNotConnected = errors.New("cache: not connected yet")

type Cache struct {
	mu     sync.RWMutex
	rdb    *redis.Client
	prefix string
}

// New returns an unconnected handle.
//
// Same reasoning as database.New: the HTTP server starts before its
// dependencies are up, so anything constructed at route-mount time must hold a
// pointer that becomes usable later rather than a nil client captured forever.
func New(appEnv config.Env) *Cache {
	return &Cache{prefix: "dawa24:" + string(appEnv) + ":"}
}

// Connect dials Redis and attaches the client to this handle.
func (c *Cache) Connect(ctx context.Context, cfg config.Redis) error {
	rdb, err := dial(ctx, cfg)
	if err != nil {
		return err
	}

	c.mu.Lock()
	old := c.rdb
	c.rdb = rdb
	c.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}
	return nil
}

// client returns the live client, or ErrNotConnected.
func (c *Cache) client() (*redis.Client, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.rdb == nil {
		return nil, ErrNotConnected
	}
	return c.rdb, nil
}

func Open(ctx context.Context, cfg config.Redis, appEnv config.Env) (*Cache, error) {
	rdb, err := dial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Cache{rdb: rdb, prefix: "dawa24:" + string(appEnv) + ":"}, nil
}

func dial(ctx context.Context, cfg config.Redis) (*redis.Client, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("cache: parse REDIS_URL: %w", err)
	}
	opts.PoolSize = cfg.PoolSize

	rdb := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("cache: ping: %w", err)
	}

	return rdb, nil
}

func (c *Cache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rdb == nil {
		return nil
	}
	err := c.rdb.Close()
	c.rdb = nil
	return err
}

func (c *Cache) Health(ctx context.Context) error {
	rdb, err := c.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return rdb.Ping(ctx).Err()
}

// Redis exposes the client for the session store and rate limiter.
func (c *Cache) Redis() *redis.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rdb
}

// Key builds a tenant-scoped cache key.
//
// Pass orgID 0 only for genuinely global data such as countries, currencies and
// platform settings. Everything else must carry its organisation.
func (c *Cache) Key(orgID int64, parts ...string) string {
	var b strings.Builder
	b.WriteString(c.prefix)
	if orgID > 0 {
		b.WriteString("org:")
		b.WriteString(strconv.FormatInt(orgID, 10))
		b.WriteByte(':')
	} else {
		b.WriteString("global:")
	}
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(p)
	}
	return b.String()
}

// GetJSON reads and decodes a value, returning ErrMiss when absent.
func (c *Cache) GetJSON(ctx context.Context, key string, dst any) error {
	rdb, err := c.client()
	if err != nil {
		// Redis being unavailable must not fail the request; the caller
		// recomputes on a miss.
		return ErrMiss
	}
	raw, err := rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return ErrMiss
	}
	if err != nil {
		return fmt.Errorf("cache: get %s: %w", key, err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		// A value we cannot decode is worse than no value: drop it so the next
		// request repopulates rather than failing forever on a stale shape.
		_ = rdb.Del(ctx, key).Err()
		return ErrMiss
	}
	return nil
}

// SetJSON encodes and stores a value with a TTL.
//
// A TTL is mandatory. Unbounded cache entries were how the legacy cache table
// grew without limit; here an omitted expiry would simply be a slow leak.
func (c *Cache) SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("cache: TTL must be positive")
	}
	raw, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("cache: marshal %s: %w", key, err)
	}

	rdb, err := c.client()
	if err != nil {
		return err
	}
	if err := rdb.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("cache: set %s: %w", key, err)
	}
	return nil
}

// Delete removes keys, used on write to invalidate.
func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	rdb, err := c.client()
	if err != nil {
		return err
	}
	return rdb.Del(ctx, keys...).Err()
}

var rememberGroup singleflight.Group

// Remember returns the cached value or computes, stores and returns it.
//
// A failure to read or write the cache is logged by the caller but never fails
// the request: the compute path is always available. Redis being down should
// make the site slow, not broken.
//
// Concurrent callers for the same uncached or expired key are coalesced via
// singleflight, preventing cache stampedes against the primary database.
func Remember[T any](ctx context.Context, c *Cache, key string, ttl time.Duration, compute func(context.Context) (T, error)) (T, error) {
	var cached T
	if err := c.GetJSON(ctx, key, &cached); err == nil {
		return cached, nil
	}

	v, err, _ := rememberGroup.Do(key, func() (any, error) {
		// Re-check under the singleflight barrier in case the winner just cached it
		var again T
		if err := c.GetJSON(ctx, key, &again); err == nil {
			return again, nil
		}

		val, cErr := compute(ctx)
		if cErr != nil {
			return val, cErr
		}
		_ = c.SetJSON(ctx, key, val, ttl)
		return val, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	res, ok := v.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("cache: unexpected type %T", v)
	}
	return res, nil
}

// InvalidateTenant drops every cached entry for one organisation.
//
// It uses SCAN rather than KEYS: KEYS blocks Redis for the duration of the
// sweep, which on a shared instance stalls every other tenant's requests too.
func (c *Cache) InvalidateTenant(ctx context.Context, orgID int64) error {
	rdb, err := c.client()
	if err != nil {
		return err
	}

	pattern := c.Key(orgID) + "*"
	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return fmt.Errorf("cache: scan %s: %w", pattern, err)
		}
		if len(keys) > 0 {
			if err := rdb.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("cache: delete batch: %w", err)
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}
