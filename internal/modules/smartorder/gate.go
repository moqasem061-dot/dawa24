package smartorder

import (
	"context"
	"time"
)

// AvailabilityGate is the platform's purchase rule, as smart ordering sees it.
// It MUST be backed by commerce.CheckAvailability — a second implementation is
// how the review screen and the checkout came to disagree.
type AvailabilityGate interface {
	Check(ctx context.Context, buyerOrgID, buyerBranchID int64, when time.Time,
		lines []GateLine) (map[int64]GateVerdict, error)
}

type GateLine struct {
	VariantID   int64
	VendorOrgID int64
	Quantity    int
}

type GateVerdict struct {
	Allowed     bool
	MaxQuantity int
	Reason      string // the commerce Reason string, carried opaquely
}

// AvailabilityFunc adapts a function into an AvailabilityGate.
type AvailabilityFunc func(ctx context.Context, buyerOrgID, buyerBranchID int64, when time.Time,
	lines []GateLine) (map[int64]GateVerdict, error)

func (f AvailabilityFunc) Check(ctx context.Context, buyerOrgID, buyerBranchID int64, when time.Time,
	lines []GateLine) (map[int64]GateVerdict, error) {
	return f(ctx, buyerOrgID, buyerBranchID, when, lines)
}
