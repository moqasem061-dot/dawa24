// Package reqcache memoises reads for the lifetime of one request.
//
// A page is assembled by independent pieces — middleware, the buying-branch
// selector, the offer rule, coverage — and each asked for the same branch row
// and the same institutional works on its own: the catalogue read one branch
// four times per request, the supplier directory seven. Caching them across
// requests would serve stale data after an edit; caching them within one
// request cannot, because the request itself is the unit of work, and a write
// made during it invalidates its key.
package reqcache

import (
	"context"
	"sync"
)

type key struct{}

type store struct {
	mu     sync.Mutex
	values map[string]any
}

// With attaches an empty cache to ctx.
func With(ctx context.Context) context.Context {
	return context.WithValue(ctx, key{}, &store{values: map[string]any{}})
}

// Get returns the value cached under k, loading it on the first call. Errors
// are not cached. Without a cache on ctx it simply calls load.
func Get[T any](ctx context.Context, k string, load func() (T, error)) (T, error) {
	s, _ := ctx.Value(key{}).(*store)
	if s == nil {
		return load()
	}
	s.mu.Lock()
	if v, ok := s.values[k]; ok {
		s.mu.Unlock()
		return v.(T), nil
	}
	s.mu.Unlock()

	v, err := load()
	if err != nil {
		return v, err
	}
	s.mu.Lock()
	s.values[k] = v
	s.mu.Unlock()
	return v, nil
}

// Forget drops keys, for a write that changes what they hold.
func Forget(ctx context.Context, keys ...string) {
	s, _ := ctx.Value(key{}).(*store)
	if s == nil {
		return
	}
	s.mu.Lock()
	for _, k := range keys {
		delete(s.values, k)
	}
	s.mu.Unlock()
}
