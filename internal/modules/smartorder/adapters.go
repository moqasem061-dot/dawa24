package smartorder

import (
	"context"
	"time"
)

// Adapters that let the composition root wire smart ordering to services owned
// by other modules without smartorder importing them.
//
// Each is a function type satisfying the interface the pipeline or finalizer
// declares. The composition root closes over the real service; tests close over
// a stub. This is what keeps AGENTS.md rule 5 — modules do not import modules —
// from turning into either a copy of the coverage rules or a cyclic import.

// CoverageFunc adapts workflow.CoverageService.ServesPoint.
type CoverageFunc func(ctx context.Context, vendorOrgID int64, day time.Weekday, lat, lng float64) (bool, int, error)

// Serves satisfies the pipeline's CoverageGate.
func (f CoverageFunc) Serves(ctx context.Context, vendorOrgID int64, day time.Weekday, lat, lng float64) (bool, int, error) {
	return f(ctx, vendorOrgID, day, lat, lng)
}

// InstitutionalCheck is everything the Corporate Operations rule needs about
// one vendor's offer.
//
// It carries the branches rather than only the product's work ids because the
// platform's rule is about branches: a pharmacy branch may buy from a supplier
// branch when the works it holds are connected to a work that branch holds.
// The gate used to be asked a narrower question — "does the buyer organisation
// hold one of this PRODUCT's works?" — which is a different rule with a
// different answer, and the disagreement only surfaced at checkout.
type InstitutionalCheck struct {
	// BuyerOrgID and BuyerBranchID are where the order is going.
	BuyerOrgID    int64
	BuyerBranchID int64
	// VendorOrgID is the supplier, and VendorBranchID the branch its offer sits
	// on. Nil means the offer names no branch, in which case every branch of
	// the supplier counts — the same reading the catalogue applies.
	VendorOrgID    int64
	VendorBranchID *int64
	VariantID      int64
	// ProductWorkIDs is catalog.products.institutional_work_ids. It is carried
	// for the Simple-mode fallback below, which is all a deployment without the
	// org service can evaluate.
	ProductWorkIDs []int64
}

// InstitutionalFunc adapts the org module's institutional gate.
type InstitutionalFunc func(ctx context.Context, c InstitutionalCheck) (bool, error)

// Visible satisfies the pipeline's InstitutionalGate.
func (f InstitutionalFunc) Visible(ctx context.Context, c InstitutionalCheck) (bool, error) {
	return f(ctx, c)
}

// BranchLocationFunc adapts the org module's branch lookup.
type BranchLocationFunc func(ctx context.Context, orgID, branchID int64) (lat, lng float64, ok bool, err error)

// Location satisfies the worker's BranchResolver.
func (f BranchLocationFunc) Location(ctx context.Context, orgID, branchID int64) (float64, float64, bool, error) {
	return f(ctx, orgID, branchID)
}

// PlaceOrderFunc adapts commerce checkout.
type PlaceOrderFunc func(ctx context.Context, req PlaceOrderRequest) (int64, error)

// PlaceOrder satisfies OrderPlacer.
func (f PlaceOrderFunc) PlaceOrder(ctx context.Context, req PlaceOrderRequest) (int64, error) {
	return f(ctx, req)
}

// SimpleInstitutionalGate intersects a product's own restriction list with a
// set of works the buyer holds.
//
// This is NOT the platform's purchase rule — that one is about branches and
// lives in org.Service.BranchesInstitutionallyConnected, which every
// composition root wires in. This is the degenerate gate for a deployment or a
// test that has no org service to ask: a product with no restriction is visible
// to everyone, and a restricted one is visible when the buyer holds one of its
// works. Using it in production would reproduce the review-passes /
// checkout-refuses split it was written before.
func SimpleInstitutionalGate(authorizedWorkIDs []int64) InstitutionalFunc {
	authorized := make(map[int64]bool, len(authorizedWorkIDs))
	for _, id := range authorizedWorkIDs {
		authorized[id] = true
	}
	return func(_ context.Context, c InstitutionalCheck) (bool, error) {
		if len(c.ProductWorkIDs) == 0 {
			return true, nil // unrestricted
		}
		for _, id := range c.ProductWorkIDs {
			if authorized[id] {
				return true, nil
			}
		}
		return false, nil
	}
}

// AlwaysInstitutionallyVisible is the gate a deployment uses when it has
// nothing to ask. It is deliberately greppable: it lets every offer through,
// and any run using it is one where checkout is the only thing enforcing
// Corporate Operations.
func AlwaysInstitutionallyVisible() InstitutionalFunc {
	return func(context.Context, InstitutionalCheck) (bool, error) { return true, nil }
}

// AlwaysCovered is the coverage gate used when the buyer's branch has no
// coordinates.
//
// Refusing every supplier because an address is incomplete would be worse than
// the alternative, and it matches how the rest of the platform behaves when
// coverage data is absent. The worker logs a warning when this path is taken, so
// it is visible rather than silent.
func AlwaysCovered() CoverageFunc {
	return func(context.Context, int64, time.Weekday, float64, float64) (bool, int, error) {
		return true, 0, nil
	}
}
