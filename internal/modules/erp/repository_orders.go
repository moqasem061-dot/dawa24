package erp

import (
	"context"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// OrderFilter is one page request over the order feeds. After is the keyset
// position (the last shipment id for sales, the last order id for purchases);
// the repository returns at most Limit rows.
type OrderFilter struct {
	After    int64
	Limit    int
	State    string
	From, To *time.Time
}

// OrderLine is one line as the ERP sees it. Purchase lines carry no external
// code (the buyer's integration does not link the supplier's variants).
type OrderLine struct {
	LineID                                int64
	VariantPublicID, ExternalProductCode  string
	Barcode                               string
	Name                                  map[string]string
	Quantity                              int
	UnitPrice, DiscountAmount, TotalPrice money.Amount
}

// CustomerInfo is the buyer as a supplier may see it. ErpCode is the customer
// code the supplier already uses (research D-20); when Mapped is false,
// UnmappedReason says why.
type CustomerInfo struct {
	ErpCode, UnmappedReason string
	Mapped                  bool
	Name, BranchName        map[string]string
	Address, Phone          string
}

// SalesRow is the supplier's own shipment of one order.
type SalesRow struct {
	OrderID, ShipmentID                                             int64
	OrderNumber, ShipmentNumber, Status, ImportState, PaymentMethod string
	PaymentStatus, Notes, ErpReference                              string
	CreatedAt, UpdatedAt                                            time.Time
	AckedAt                                                         *time.Time
	Subtotal, ShippingFee, Total                                    money.Amount
	Customer                                                        CustomerInfo
	Lines                                                           []OrderLine
}

// PurchaseShipment is one supplier's shipment inside a buyer's order.
type PurchaseShipment struct {
	OrderID                int64
	ShipmentNumber, Status string
	SupplierName           map[string]string
	Subtotal, ShippingFee  money.Amount
	Lines                  []OrderLine
}

// PurchaseRow is one buyer order, headers only until the shipments are joined.
type PurchaseRow struct {
	OrderID                          int64
	OrderNumber, Status, ImportState string
	ExternalReference, ErpReference  string
	CreatedAt, UpdatedAt             time.Time
	AckedAt                          *time.Time
	Total                            money.Amount
	Shipments                        []PurchaseShipment
}

// OrderImport is one acknowledgement to store. ShipmentID is set for sales.
// OrganizationID is the tenant the row belongs to (RLS needs it on every write).
type OrderImport struct {
	OrganizationID, IntegrationID, OrderID, APIKeyID int64
	ShipmentID                                       *int64
	Direction, OrderNumber                           string
	ErpReference, AckedStatus                        string
}

// AckOutcome is the repository's answer for one acknowledgement. Result is
// acknowledged, already_acknowledged or conflict.
type AckOutcome struct {
	Result, Code, ExistingReference string
}

// OrderRepository is the persistence of the S3 order feeds and acknowledgements.
type OrderRepository interface {
	ListSalesShipmentsAsSystem(ctx context.Context, orgID, integrationID int64, f OrderFilter) ([]SalesRow, error)
	GetSalesShipmentAsSystem(ctx context.Context, orgID int64, orderNumber string) (SalesRow, error)
	ListPurchaseOrders(ctx context.Context, buyerOrgID, integrationID int64, f OrderFilter) ([]PurchaseRow, error)
	GetPurchaseOrder(ctx context.Context, buyerOrgID int64, orderNumber string) (PurchaseRow, error)
	ListPurchaseShipmentsAsSystem(ctx context.Context, buyerOrgID int64, orderIDs []int64) ([]PurchaseShipment, error)
	UpsertOrderImport(ctx context.Context, imp OrderImport, audit AuditRecord) (AckOutcome, error)
}
