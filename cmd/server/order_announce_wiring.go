package main

import (
	"context"
	"sync"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
)

// The API's commerce service (JSON checkout, Smart Ordering) is built inside
// the API group, before the UI handler that owns notification fan-out exists.
// It is handed this proxy, bound once the handler is built, so an order placed
// through either service is announced by the same code.
type lateOrderAnnouncer struct {
	mu sync.RWMutex
	fn commerce.OrderPlacedHook
}

func (l *lateOrderAnnouncer) bind(fn commerce.OrderPlacedHook) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.fn = fn
	l.mu.Unlock()
}

// announce is a commerce.OrderPlacedHook.
func (l *lateOrderAnnouncer) announce(ctx context.Context, order *commerce.Order) {
	if l == nil {
		return
	}
	l.mu.RLock()
	fn := l.fn
	l.mu.RUnlock()
	if fn != nil {
		fn(ctx, order)
	}
}
