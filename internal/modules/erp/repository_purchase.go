package erp

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// PurchaseRepository is the persistence of S7 purchase orders: the supplier
// offers the buyer may order from, the per-key idempotency record and the link
// check. The offer reads are named exceptions E6 (plan.md); the idempotency and
// link reads run under the buyer's tenant.
type PurchaseRepository interface {
	// ListOffersAsSystem is E6: supplier variants with the barcode, public fields only.
	ListOffersAsSystem(ctx context.Context, buyerOrgID int64, barcode string, limit int) ([]Offer, error)
	// VariantOfferAsSystem is E6: one variant by its public id, with the flags the
	// purchase rules need.
	VariantOfferAsSystem(ctx context.Context, publicID string) (VariantOfferRow, error)
	// BeginIdempotency inserts the in_progress row, or reads the row that exists.
	BeginIdempotency(ctx context.Context, orgID, integrationID int64, ref, requestSHA string) (IdemState, error)
	// FinishIdempotency stores the response and writes the audit row in one transaction.
	FinishIdempotency(ctx context.Context, orgID, integrationID int64, ref string, orderID int64, orderNumber string, response []byte, audit AuditRecord) error
	DeleteIdempotency(ctx context.Context, orgID, integrationID int64, ref string) error
	// FindOrderByNoteMarker finds the buyer's order whose note carries the marker.
	FindOrderByNoteMarker(ctx context.Context, buyerOrgID int64, marker string) (orderID int64, orderNumber string, found bool, err error)
	// SupplierLinked reports whether the buyer has a pending or approved link to the supplier.
	SupplierLinked(ctx context.Context, buyerOrgID, supplierOrgID int64) (bool, error)
}

// Offer is one supplier product the buyer may order, as GET /offers shows it.
type Offer struct {
	VariantID                    string
	SupplierName, Name           map[string]string
	SupplierLinked               bool
	Barcode                      string
	PublicPrice, DiscountPercent money.Amount
	UnitPrice                    money.Amount
	InStock                      bool
}

// VariantOfferRow is one variant as the purchase rules see it. Active is true
// when the variant is live and its supplier is an approved vendor; InStock is
// true when some warehouse holds a positive balance.
type VariantOfferRow struct {
	ID, OrganizationID           int64
	PublicID                     string
	PublicPrice, DiscountPercent money.Amount
	SupplierName                 map[string]string
	Active, InStock              bool
}

// IdemState is what BeginIdempotency found. Created means the caller owns a new
// in_progress row. Otherwise RequestSHA is the stored request fingerprint, and
// Replay (state done), InProgress, or Stale (in_progress older than the stale
// window) says what to do. Response is the stored answer when Replay is set.
type IdemState struct {
	Created, Replay, InProgress, Stale bool
	RequestSHA                         string
	Response                           []byte
}

// CreatePurchaseRequest is the body of POST /orders/purchase after decoding.
type CreatePurchaseRequest struct {
	ExternalReference string
	Notes             string
	Lines             []PurchaseLine
}

// PurchaseLine is one requested line. VariantID is the public id; UnitPrice is
// the optional price the ERP sent, compared with Dawa24's.
type PurchaseLine struct {
	VariantID string
	Quantity  int
	UnitPrice *money.Amount
}

// PurchaseResult is the order the call answers with. Replay is true when the
// reference was seen before and the stored order is returned.
type PurchaseResult struct {
	OrderID     int64
	OrderNumber string
	Replay      bool
	Row         PurchaseRow
}
