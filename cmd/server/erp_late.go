package main

import (
	"context"
	"sync"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

// lateERPDecider is the erp.OrderDecider the ERP API holds from the start. The
// website's UI handler, which carries the decision rules, is built after the ERP
// routes are mounted, so the method is bound once the handler exists (the pattern
// of lateOrderAnnouncer). A call before the bind is refused as unavailable.
type lateERPDecider struct {
	mu sync.RWMutex
	fn func(context.Context, erp.ShipmentDecision) (erp.ShipmentDecisionResult, error)
}

var _ erp.OrderDecider = (*lateERPDecider)(nil)

func (l *lateERPDecider) bind(fn func(context.Context, erp.ShipmentDecision) (erp.ShipmentDecisionResult, error)) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.fn = fn
	l.mu.Unlock()
}

// Decide implements erp.OrderDecider.
func (l *lateERPDecider) Decide(ctx context.Context, d erp.ShipmentDecision) (erp.ShipmentDecisionResult, error) {
	if l == nil {
		return erp.ShipmentDecisionResult{}, erp.NewError("ERP_UNAVAILABLE", 503, nil)
	}
	l.mu.RLock()
	fn := l.fn
	l.mu.RUnlock()
	if fn == nil {
		return erp.ShipmentDecisionResult{}, erp.NewError("ERP_UNAVAILABLE", 503, nil)
	}
	return fn(ctx, d)
}

// lateERPPurchasePlacer is the erp.PurchasePlacer the ERP API holds from the
// start. The placer needs the billing and commerce services, which are built
// after the ERP routes are mounted, so it is bound once they exist. A call before
// the bind is refused as unavailable.
type lateERPPurchasePlacer struct {
	mu     sync.RWMutex
	placer erp.PurchasePlacer
}

var _ erp.PurchasePlacer = (*lateERPPurchasePlacer)(nil)

func (l *lateERPPurchasePlacer) bind(p erp.PurchasePlacer) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.placer = p
	l.mu.Unlock()
}

// Place implements erp.PurchasePlacer.
func (l *lateERPPurchasePlacer) Place(ctx context.Context, req erp.PurchaseRequest) (erp.PlacedOrder, error) {
	if l == nil {
		return erp.PlacedOrder{}, erp.NewError("ERP_UNAVAILABLE", 503, nil)
	}
	l.mu.RLock()
	p := l.placer
	l.mu.RUnlock()
	if p == nil {
		return erp.PlacedOrder{}, erp.NewError("ERP_UNAVAILABLE", 503, nil)
	}
	return p.Place(ctx, req)
}
