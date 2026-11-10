package erp

import (
	"context"
	"time"
)

// The integration activity screen (plan section 3). Every read here is tenant
// scoped: the repository runs it in a read transaction with the organization's
// GUC set and filters by organization_id = $1 as well.

const (
	// CallsDefaultLimit and CallsMaxLimit bound one page of the call log.
	CallsDefaultLimit = 50
	CallsMaxLimit     = 200
	// BatchesDefaultLimit and BatchesMaxLimit bound one page of batches.
	BatchesDefaultLimit = 50
	BatchesMaxLimit     = 200
	// ActivityImportCap caps the order counts: the screen says "500 or more"
	// rather than counting a whole history on every page view.
	ActivityImportCap = 500
	// ActivityLiveBatchLimit caps the live panel, which shows active batches.
	ActivityLiveBatchLimit = 50
	// ActivityOrdersLimit is the orders tab's page.
	ActivityOrdersLimit = 50
)

// ActivityCounts is the one-read summary of the organization's integration:
// the status, the key count, the last call, the last 24 hours of calls and
// batches, and the open review items.
type ActivityCounts struct {
	IntegrationID     int64
	IntegrationStatus string
	ActiveKeys        int
	LastCallAt        *time.Time
	LastCallStatus    int
	LastCallRoute     string
	Calls24h          int
	Errors24h         int
	AvgDurationMS     int
	BatchesQueued     int
	BatchesProcessing int
	BatchesFailed24h  int
	OpenReview        int
}

// ActivitySummary adds the order counts to ActivityCounts. The counts stop at
// ActivityImportCap.
type ActivitySummary struct {
	ActivityCounts
	SalesNew, SalesChanged       int
	PurchaseNew, PurchaseChanged int
}

// CallFilter selects calls. Result is "all", "ok" (status below 400) or "error"
// (status 400 and up); BeforeID pages backwards from a call id.
type CallFilter struct {
	BeforeID         int64
	Result           string
	KeyPrefix, Route string
	From, To         *time.Time
	Limit            int
}

// CallRow is one call-log entry with the key's name (empty when the key is gone).
type CallRow struct {
	ID            int64
	CreatedAt     time.Time
	KeyName       string
	KeyPrefix     string
	Method, Route string
	Status        int
	ErrorCode     string
	DurationMS    int
	RequestID     string
	Query         string
	UserAgent     string
	IP            string
	BytesIn       int
	BytesOut      int
	Ref           string
	AuthReason    string
	ErrorDetail   string
}

// BatchRow is a product-sync batch with the name of the key that sent it.
type BatchRow struct {
	SyncBatch
	KeyName string
}

// OrderImportRow is one acknowledged order, as the orders tab lists it.
type OrderImportRow struct {
	Direction, OrderNumber, ErpReference, AckedStatus string
	OrderID                                           int64
	AckedAt                                           time.Time
	AckCount                                          int
}

// ActivitySummary reads the screen's numbers. The order counts reuse the feed
// reads with the "new" and "changed" import states, so they need no new
// named exception.
func (s *Service) ActivitySummary(ctx context.Context, orgID int64) (ActivitySummary, error) {
	counts, err := s.repo.ActivityCounts(ctx, orgID)
	if err != nil {
		return ActivitySummary{}, err
	}
	out := ActivitySummary{ActivityCounts: counts}
	if counts.IntegrationID == 0 {
		return out, nil
	}
	for _, state := range []string{"new", "changed"} {
		filter := OrderFilter{State: state, Limit: ActivityImportCap + 1}
		sales, err := s.repo.ListSalesShipmentsAsSystem(ctx, orgID, counts.IntegrationID, filter)
		if err != nil {
			return ActivitySummary{}, NewError("ERP_INTERNAL", 500, nil)
		}
		purchase, err := s.repo.ListPurchaseOrders(ctx, orgID, counts.IntegrationID, filter)
		if err != nil {
			return ActivitySummary{}, NewError("ERP_INTERNAL", 500, nil)
		}
		if state == "new" {
			out.SalesNew, out.PurchaseNew = capCount(len(sales)), capCount(len(purchase))
		} else {
			out.SalesChanged, out.PurchaseChanged = capCount(len(sales)), capCount(len(purchase))
		}
	}
	return out, nil
}

func capCount(n int) int {
	if n > ActivityImportCap {
		return ActivityImportCap
	}
	return n
}

// ListCalls pages the organization's call log, newest first. An unknown result
// filter reads as "all"; the limit is clamped to the default and the maximum.
func (s *Service) ListCalls(ctx context.Context, orgID int64, f CallFilter) ([]CallRow, error) {
	switch f.Result {
	case "ok", "error":
	default:
		f.Result = "all"
	}
	f.Limit = clampLimit(f.Limit, CallsDefaultLimit, CallsMaxLimit)
	if f.BeforeID < 0 {
		f.BeforeID = 0
	}
	return s.repo.ListCalls(ctx, orgID, f)
}

// ListBatches pages the organization's sync batches, newest first.
func (s *Service) ListBatches(ctx context.Context, orgID, beforeID int64, limit int) ([]BatchRow, error) {
	if beforeID < 0 {
		beforeID = 0
	}
	return s.repo.ListBatches(ctx, orgID, beforeID, clampLimit(limit, BatchesDefaultLimit, BatchesMaxLimit))
}

// ListActiveBatches returns the batches still queued or processing.
func (s *Service) ListActiveBatches(ctx context.Context, orgID int64) ([]BatchRow, error) {
	return s.repo.ListActiveBatches(ctx, orgID, ActivityLiveBatchLimit)
}

// ListOrderImports returns the latest acknowledged orders of both directions.
func (s *Service) ListOrderImports(ctx context.Context, orgID int64) ([]OrderImportRow, error) {
	return s.repo.ListOrderImports(ctx, orgID, ActivityOrdersLimit)
}

// clampLimit gives the default for a zero or negative limit and the maximum
// for anything above it.
func clampLimit(limit, def, max int) int {
	switch {
	case limit <= 0:
		return def
	case limit > max:
		return max
	}
	return limit
}
