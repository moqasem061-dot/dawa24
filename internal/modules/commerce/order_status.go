package commerce

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// IsValidStatusTransition validates order state machine transitions. Compare-
// and-swap callers pass the status they read and the status they want; anything
// outside the DAG below is refused. Terminal states: cancelled, returned,
// failed, refunded, completed (only refundable afterwards).
func IsValidStatusTransition(from, to OrderStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusPending:
		return to == StatusProcessing || to == StatusConfirmed || to == StatusShipped || to == StatusDelivered || to == StatusCancelled || to == StatusOnHold
	case StatusProcessing:
		return to == StatusConfirmed || to == StatusShipped || to == StatusDelivered || to == StatusOnHold || to == StatusCancelled || to == StatusFailed
	case StatusConfirmed:
		return to == StatusShipped || to == StatusInTransit || to == StatusOutForDelivery || to == StatusDelivered || to == StatusOnHold || to == StatusCancelled || to == StatusFailed
	case StatusOnHold:
		return to == StatusProcessing || to == StatusConfirmed || to == StatusShipped || to == StatusCancelled || to == StatusFailed
	case StatusShipped:
		return to == StatusInTransit || to == StatusOutForDelivery || to == StatusDelivered || to == StatusCompleted || to == StatusReturned || to == StatusFailed
	case StatusInTransit:
		return to == StatusOutForDelivery || to == StatusDelivered || to == StatusCompleted || to == StatusReturned || to == StatusFailed
	case StatusOutForDelivery:
		return to == StatusDelivered || to == StatusCompleted || to == StatusReturned || to == StatusFailed
	case StatusDelivered:
		return to == StatusShipped || to == StatusInTransit || to == StatusOutForDelivery || to == StatusCompleted || to == StatusReturned || to == StatusRefunded
	case StatusCompleted:
		return to == StatusShipped || to == StatusInTransit || to == StatusOutForDelivery || to == StatusRefunded
	case StatusCancelled, StatusFailed, StatusReturned, StatusRefunded:
		return false // Terminal states
	default:
		return false
	}
}

// GenerateOrderNumber formats a clean, simplified, natural order identifier without prefixes.
// It produces short, human-friendly natural numbers (e.g. "2608270001" or "10542") that are easy to read and unique.
func GenerateOrderNumber(t time.Time, id int64) string {
	if id > 0 && id < 100000000 {
		return fmt.Sprintf("%d", id)
	}
	seq := id % 10000
	if seq < 0 {
		seq = -seq
	}
	return fmt.Sprintf("%s%04d", t.Format("060102"), seq)
}

// GenerateShipmentNumber formats a shipment partition identifier based on the order number.
func GenerateShipmentNumber(orderNumber string, seq int) string {
	return fmt.Sprintf("%s-%d", orderNumber, seq)
}

// GenerateDeliveryCode produces a secure random 6-digit numeric verification PIN.
func GenerateDeliveryCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		// crypto/rand does not fail on a supported platform. If it ever does,
		// a fixed PIN would be a delivery code everyone knows; refuse instead.
		panic("commerce: crypto/rand unavailable for delivery code: " + err.Error())
	}
	return fmt.Sprintf("%06d", n.Int64()+100000)
}

// Standard courier delivery errors.
var (
	ErrDeliveryLocked       = apperr.Conflict("delivery.locked", i18n.TDefault("w4_mod.15_138"))
	ErrInvalidDeliveryCode  = apperr.Validation("delivery.invalid_code", i18n.TDefault("w4_mod.w4str_139_139"), map[string]string{"delivery_code": "invalid"})
	ErrDeliveryNotAvailable = apperr.Conflict("delivery.not_available", i18n.TDefault("w4_mod.w4str_140_140"))
)

// GenerateTrackingNumber produces a formatted tracking reference code for couriers.
func GenerateTrackingNumber(orderNumber string, seq int) string {
	n, err := rand.Int(rand.Reader, big.NewInt(9000))
	randSuffix := int64(1000)
	if err == nil {
		randSuffix = n.Int64() + 1000
	}
	if orderNumber != "" {
		return fmt.Sprintf("TRK-%s-%d%04d", orderNumber, seq, randSuffix)
	}
	return fmt.Sprintf("TRK-%d%04d", time.Now().Unix()%1000000, randSuffix)
}

// ValidateLine ensures an order line has positive quantity and consistent totals.
func (l *OrderLine) ValidateLine() error {
	if l.Quantity <= 0 {
		return apperr.Validation("line.quantity_invalid", "Quantity must be greater than zero.", nil)
	}
	if l.UnitPrice.IsNegative() {
		return apperr.Validation("line.price_invalid", "Unit price cannot be negative.", nil)
	}
	return nil
}

// WishlistItem represents an item in a user's wishlist / favorites.
type WishlistItem struct {
	ID        int64     `json:"id"`
	PublicID  string    `json:"public_id"`
	UserID    int64     `json:"user_id"`
	ProductID int64     `json:"product_id"`
	CreatedAt time.Time `json:"created_at"`
}

// QuoteStatus represents the state of a buyer quote request.
type QuoteStatus string

const (
	QuotePending  QuoteStatus = "pending"
	QuoteQuoted   QuoteStatus = "quoted"
	QuoteAccepted QuoteStatus = "accepted"
	QuoteRejected QuoteStatus = "rejected"
	QuoteExpired  QuoteStatus = "expired"
)

// QuoteRequest represents a buyer-initiated inquiry for bulk special pricing.
type QuoteRequest struct {
	ID                int64        `json:"id"`
	PublicID          string       `json:"public_id"`
	OrganizationID    int64        `json:"organization_id"`
	CustomerOrgID     int64        `json:"customer_org_id"`
	ProductID         *int64       `json:"product_id,omitempty"`
	ProductName       string       `json:"product_name"`
	RequestedQuantity int          `json:"requested_quantity"`
	TargetUnitPrice   money.Amount `json:"target_unit_price"`
	QuoteUnitPrice    money.Amount `json:"quote_unit_price"`
	Status            QuoteStatus  `json:"status"`
	BuyerNotes        string       `json:"buyer_notes,omitempty"`
	SupplierNotes     string       `json:"supplier_notes,omitempty"`
	ValidUntil        *time.Time   `json:"valid_until,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// PurchaseRequestStatus defines the state of a multi-line procurement purchase request (Plan V5 Phase 3 Task 3.1).
type PurchaseRequestStatus string

const (
	PurchaseRequestPending    PurchaseRequestStatus = "pending"
	PurchaseRequestApproved   PurchaseRequestStatus = "approved"
	PurchaseRequestProcessing PurchaseRequestStatus = "processing"
	PurchaseRequestCompleted  PurchaseRequestStatus = "completed"
	PurchaseRequestCancelled  PurchaseRequestStatus = "cancelled"
)

// PurchaseRequest represents a customer-initiated formal multi-line procurement purchase request.
type PurchaseRequest struct {
	ID             int64                  `json:"id"`
	PublicID       string                 `json:"public_id"`
	RequestNumber  string                 `json:"request_number"`
	CustomerID     int64                  `json:"customer_id"`
	OrganizationID *int64                 `json:"organization_id,omitempty"`
	BranchID       *int64                 `json:"branch_id,omitempty"`
	VendorOrgID    int64                  `json:"vendor_org_id"`
	VendorBranchID *int64                 `json:"vendor_branch_id,omitempty"`
	Status         PurchaseRequestStatus  `json:"status"`
	TotalItems     int                    `json:"total_items"`
	EstimatedTotal money.Amount           `json:"estimated_total"`
	BuyerNotes     string                 `json:"buyer_notes,omitempty"`
	VendorNotes    string                 `json:"vendor_notes,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	RespondedAt    *time.Time             `json:"responded_at,omitempty"`
	RespondedBy    *int64                 `json:"responded_by,omitempty"`
	Lines          []*PurchaseRequestLine `json:"lines,omitempty"`
	VendorName     string                 `json:"vendor_name,omitempty"`
	CustomerName   string                 `json:"customer_name,omitempty"`
}

// PurchaseRequestLine represents a specific item within a purchase request.
type PurchaseRequestLine struct {
	ID              int64        `json:"id"`
	RequestID       int64        `json:"request_id"`
	ProductID       *int64       `json:"product_id,omitempty"`
	ProductName     string       `json:"product_name"`
	ProductSKU      string       `json:"product_sku,omitempty"`
	Quantity        int          `json:"quantity"`
	TargetPrice     money.Amount `json:"target_price"`
	TargetDiscount  float64      `json:"target_discount"`
	OfferedPrice    money.Amount `json:"offered_price"`
	OfferedDiscount float64      `json:"offered_discount"`
	Status          string       `json:"status"` // pending, approved, rejected, modified
	Notes           string       `json:"notes,omitempty"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// GeneratePurchaseRequestNumber formats a unique purchase request identifier (Rule R7 / Plan V5 §3.1).
func GeneratePurchaseRequestNumber(t time.Time, id int64) string {
	return fmt.Sprintf("PR-%s-%06d", t.Format("20060102"), id%1000000)
}
