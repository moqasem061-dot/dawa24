package commerce

import (
	"context"
	"fmt"
)

// OrderPlacedHook is told about every order Checkout commits.
//
// Orders are placed from five places — the checkout page, Capsule's
// place_order action, the negotiation form, Smart Ordering and the JSON API —
// and only the checkout page used to announce one. A supplier restocking
// through Smart Ordering, or an order placed from Telegram, wrote no
// notification for the buyer, the supplier or the platform, so nothing
// reached the feed or the chat outbox either. Announcing from Checkout itself
// makes "an order exists" and "an order was announced" the same event.
//
// A hook runs after the order row is committed, with a context that outlives
// the request. It must not block: hand slow work to a goroutine.
type OrderPlacedHook func(ctx context.Context, order *Order)

// OnOrderPlaced registers a hook. Call it while wiring, before serving.
func (s *Service) OnOrderPlaced(fn OrderPlacedHook) {
	if fn != nil {
		s.placedHooks = append(s.placedHooks, fn)
	}
}

// announcePlaced runs every hook. A panicking hook is logged and contained: the
// order is already committed, and a notification bug must not turn a placed
// order into a failed checkout.
func (s *Service) announcePlaced(ctx context.Context, order *Order) {
	if order == nil {
		return
	}
	detached := context.WithoutCancel(ctx)
	for _, fn := range s.placedHooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.log.ErrorContext(detached, "order placed hook panicked",
						"order_id", order.ID, "panic", fmt.Sprint(r))
				}
			}()
			fn(detached, order)
		}()
	}
}
