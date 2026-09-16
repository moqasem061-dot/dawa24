// Package inventory handles warehouse management, stock levels, stock movement ledgers,
// and inter-warehouse inventory transfers.
package inventory

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// MovementType classifies reasons for stock changes in the append-only ledger.
type MovementType string

const (
	MovementIn           MovementType = "in"
	MovementOut          MovementType = "out"
	MovementTransfer     MovementType = "transfer"
	MovementAdjustment   MovementType = "adjustment"
	MovementOrderReserve MovementType = "order_reserve"
	MovementOrderRelease MovementType = "order_release"
)

// TransferStatus tracks the state of an inventory transfer between two warehouses.
type TransferStatus string

const (
	TransferPending   TransferStatus = "pending"
	TransferInTransit TransferStatus = "in_transit"
	TransferCompleted TransferStatus = "completed"
	TransferCancelled TransferStatus = "cancelled"
)

// Warehouse represents a physical or logical storage facility owned by a tenant.
type Warehouse struct {
	ID             int64      `json:"id"`
	PublicID       string     `json:"public_id"`
	OrganizationID int64      `json:"organization_id"`
	BranchID       *int64     `json:"branch_id,omitempty"`
	BranchName     string     `json:"branch_name,omitempty"`
	Name           string     `json:"name"`
	Code           string     `json:"code,omitempty"`
	Address        string     `json:"address,omitempty"`
	Phone          string     `json:"phone,omitempty"`
	Latitude       *float64   `json:"latitude,omitempty"`
	Longitude      *float64   `json:"longitude,omitempty"`
	IsActive       bool       `json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

// Stock tracks the quantity of a specific product variant at a specific warehouse.
// Note: Fixes legacy defect D4 by scoping uniqueness to (warehouse_id, product_variant_id).
type Stock struct {
	ID               int64      `json:"id"`
	OrganizationID   int64      `json:"organization_id"`
	WarehouseID      int64      `json:"warehouse_id"`
	ProductID        int64      `json:"product_id"`
	ProductVariantID int64      `json:"product_variant_id"`
	Quantity         int        `json:"quantity"`
	MinThreshold     int        `json:"min_threshold"`
	Negotiation      int        `json:"negotiation"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

// StockMovement is an immutable append-only ledger entry recording inventory changes.
type StockMovement struct {
	ID             int64        `json:"id"`
	OrganizationID int64        `json:"organization_id"`
	StockID        int64        `json:"stock_id"`
	Type           MovementType `json:"type"`
	QuantityDelta  int          `json:"quantity_delta"`
	BalanceAfter   int          `json:"balance_after"`
	Details        string       `json:"details,omitempty"`
	ReferenceType  string       `json:"reference_type,omitempty"`
	ReferenceID    *int64       `json:"reference_id,omitempty"`
	UserID         *int64       `json:"user_id,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
}

// WarehouseTransfer represents a stock movement between two distinct warehouses.
type WarehouseTransfer struct {
	ID               int64          `json:"id"`
	OrganizationID   int64          `json:"organization_id"`
	FromWarehouseID  int64          `json:"from_warehouse_id"`
	ToWarehouseID    int64          `json:"to_warehouse_id"`
	ProductID        int64          `json:"product_id"`
	ProductVariantID int64          `json:"product_variant_id"`
	Quantity         int            `json:"quantity"`
	Status           TransferStatus `json:"status"`
	InitiatedBy      *int64         `json:"initiated_by,omitempty"`
	Notes            string         `json:"notes,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// Validate ensures transfer parameters meet core invariants.
func (t *WarehouseTransfer) Validate() error {
	if t.FromWarehouseID <= 0 || t.ToWarehouseID <= 0 {
		return apperr.Validation("transfer.warehouse_required", "Source and destination warehouses are required.", nil)
	}
	if t.FromWarehouseID == t.ToWarehouseID {
		return apperr.Validation("transfer.same_warehouse", "Source and destination warehouses must be different.", nil)
	}
	if t.Quantity <= 0 {
		return apperr.Validation("transfer.quantity_positive", "Transfer quantity must be greater than zero.", nil)
	}
	return nil
}

// DetailedWarehouseStockView provides comprehensive product, batch, and inventory view for warehouse modals.
type DetailedWarehouseStockView struct {
	StockID          int64      `json:"stock_id"`
	WarehouseID      int64      `json:"warehouse_id"`
	OrganizationID   int64      `json:"organization_id"`
	ProductID        int64      `json:"product_id"`
	ProductVariantID int64      `json:"product_variant_id"`
	ProductName      string     `json:"product_name"`
	VariantName      string     `json:"variant_name"`
	ScientificName   string     `json:"scientific_name,omitempty"`
	DosageForm       string     `json:"dosage_form,omitempty"`
	Concentration    string     `json:"concentration,omitempty"`
	Manufacturer     string     `json:"manufacturer,omitempty"`
	SKU              string     `json:"sku"`
	Barcode          string     `json:"barcode"`
	BatchNumber      string     `json:"batch_number"`
	ExpiryDate       *time.Time `json:"expiry_date,omitempty"`
	PriceStr         string     `json:"price_str"`
	CostPriceStr     string     `json:"cost_price_str,omitempty"`
	PublicPriceStr   string     `json:"public_price_str,omitempty"`
	DiscountStr      string     `json:"discount_str"`
	Quantity         int        `json:"quantity"`
	MinThreshold     int        `json:"min_threshold"`
	IsNegotiable     bool       `json:"is_negotiable"`
	Status           string     `json:"status"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
