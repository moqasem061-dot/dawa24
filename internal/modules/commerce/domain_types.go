package commerce

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// OrderStatus defines the canonical lifecycle states, matching Laravel's
// main_orders/adv_orders enum exactly (Rebuild V2 §3.3).
type OrderStatus string

const (
	StatusPending        OrderStatus = "pending"
	StatusProcessing     OrderStatus = "processing"
	StatusConfirmed      OrderStatus = "confirmed"
	StatusOnHold         OrderStatus = "on_hold"
	StatusShipped        OrderStatus = "shipped"
	StatusInTransit      OrderStatus = "in_transit"
	StatusOutForDelivery OrderStatus = "out_for_delivery"
	StatusDelivered      OrderStatus = "delivered"
	StatusCompleted      OrderStatus = "completed"
	StatusCancelled      OrderStatus = "cancelled"
	StatusFailed         OrderStatus = "failed"
	StatusReturned       OrderStatus = "returned"
	StatusRefunded       OrderStatus = "refunded"
)

// PaymentStatus tracks payment authorization and settlement states.
type PaymentStatus string

const (
	PaymentUnpaid            PaymentStatus = "unpaid"
	PaymentAuthorized        PaymentStatus = "authorized"
	PaymentPaid              PaymentStatus = "paid"
	PaymentPartiallyRefunded PaymentStatus = "partially_refunded"
	PaymentRefunded          PaymentStatus = "refunded"
	PaymentFailed            PaymentStatus = "failed"
)

// Cart represents an active shopping cart for a user.
type Cart struct {
	ID             int64       `json:"id"`
	PublicID       string      `json:"public_id"`
	UserID         int64       `json:"user_id"`
	OrganizationID *int64      `json:"organization_id,omitempty"`
	Items          []*CartItem `json:"items,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// CartItem represents an item within a shopping cart.
type CartItem struct {
	ID               int64        `json:"id"`
	CartID           int64        `json:"cart_id"`
	ProductID        int64        `json:"product_id"`
	ProductVariantID int64        `json:"product_variant_id"`
	OrganizationID   int64        `json:"organization_id"`
	ProductName      i18n.Text    `json:"product_name,omitempty"`
	SupplierName     i18n.Text    `json:"supplier_name,omitempty"`
	MinOrderPrice    money.Amount `json:"min_order_price,omitempty"`
	OfferID          *int64       `json:"offer_id,omitempty"` // offer the item was added under (064)
	Quantity         int          `json:"quantity"`
	AvailableStock   int          `json:"available_stock,omitempty"`
	UnitPrice        money.Amount `json:"unit_price"`
	IsCovered        bool         `json:"is_covered,omitempty"`
	CoverageReason   string       `json:"coverage_reason,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

// Order represents a master customer order placed against one offer
// (main_orders parity, Rebuild V2 §3.3).
type Order struct {
	// LineCount is filled by listings that show a count without loading lines.
	LineCount         int              `json:"line_count,omitempty"`
	ID                int64            `json:"id"`
	PublicID          string           `json:"public_id"`
	OrderNumber       string           `json:"order_number"`
	CustomerID        int64            `json:"customer_id"`
	OrganizationID    *int64           `json:"organization_id,omitempty"`
	OfferID           int64            `json:"offer_id"`                   // the offer this order belongs to (063)
	BranchID          *int64           `json:"branch_id,omitempty"`        // customer branch buying for
	VendorBranchID    *int64           `json:"vendor_branch_id,omitempty"` // fulfilling vendor branch
	UserAddressID     *int64           `json:"user_address_id,omitempty"`
	Status            OrderStatus      `json:"status"`
	Subtotal          money.Amount     `json:"subtotal"`
	DiscountAmount    money.Amount     `json:"discount_amount"`
	TotalDiscount     money.Amount     `json:"total_discount"` // offer discounts over lines (063)
	ShippingFee       money.Amount     `json:"shipping_fee"`
	TaxAmount         money.Amount     `json:"tax_amount"`
	TotalAmount       money.Amount     `json:"total_amount"`
	FinalPrice        money.Amount     `json:"final_price"` // paid after discount (063)
	PaymentMethod     string           `json:"payment_method"`
	PaymentStatus     PaymentStatus    `json:"payment_status"`
	Notes             string           `json:"notes,omitempty"`
	IsNegotiation     bool             `json:"is_negotiation"`
	NegotiationStatus string           `json:"negotiation_status"` // "none", "pending", "accepted", "rejected"
	NegotiationNotes  string           `json:"negotiation_notes,omitempty"`
	Rating            *float64         `json:"rating,omitempty"`
	Review            *string          `json:"review,omitempty"`
	RatedAt           *time.Time       `json:"rated_at,omitempty"`
	DeliveredAt       *time.Time       `json:"delivered_at,omitempty"`
	Shipments         []*OrderShipment `json:"shipments,omitempty"`
	Lines             []*OrderLine     `json:"lines,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	DeletedAt         *time.Time       `json:"deleted_at,omitempty"`

	// Enriched metadata for views
	CustomerOrgName       i18n.Text `json:"customer_org_name,omitempty"`
	CustomerBranchName    i18n.Text `json:"customer_branch_name,omitempty"`
	CustomerBranchAddress string    `json:"customer_branch_address,omitempty"`
	CustomerBranchPhone   string    `json:"customer_branch_phone,omitempty"`
	CustomerManagerName   string    `json:"customer_manager_name,omitempty"`
	CustomerCityName      string    `json:"customer_city_name,omitempty"`
	VendorOrgName         i18n.Text `json:"vendor_org_name,omitempty"`
	VendorBranchName      i18n.Text `json:"vendor_branch_name,omitempty"`
	ItemsCount            int       `json:"items_count,omitempty"`
}

// AdminOrderFilter defines comprehensive parameters for admin order searches and filtering.
type AdminOrderFilter struct {
	Query         string
	Tab           string // "all", "direct", "negotiations"
	Status        string
	PaymentStatus string
	CustomerOrgID int64
	VendorOrgID   int64
	// BuyerType narrows to the kind of company that placed the order.
	//
	// It exists because a supplier restocking from another supplier is an
	// ordinary order on this marketplace, not an edge case. The screen used to
	// call this filter "the pharmacy" and populate it only with organisations
	// of type customer, so those orders were unfindable by their buyer.
	BuyerType string
	DateFrom  string
	DateTo    string
	Limit     int
	Offset    int
}

// AdminOrderParty is one organisation that appears on an order, for the buyer
// and seller filter selects.
//
// The selects used to list every organisation on the platform and filter by
// type in the template. Listing only the parties that actually appear on an
// order is a shorter list that answers more questions, and it cannot omit a
// buyer because of its type.
type AdminOrderParty struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// AdminOrderParties is the two filter selects' contents.
type AdminOrderParties struct {
	Buyers  []AdminOrderParty `json:"buyers"`
	Sellers []AdminOrderParty `json:"sellers"`
}

// AdminOrderKPIs aggregates order metrics across the entire platform.
type AdminOrderKPIs struct {
	TotalOrders       int          `json:"total_orders"`
	DirectOrders      int          `json:"direct_orders"`
	NegotiationOrders int          `json:"negotiation_orders"`
	PendingOrders     int          `json:"pending_orders"`
	ShippingOrders    int          `json:"shipping_orders"`
	CompletedOrders   int          `json:"completed_orders"`
	CancelledOrders   int          `json:"cancelled_orders"`
	TotalVolume       money.Amount `json:"total_volume"`
}

// OrderShipment represents a vendor-specific shipment split from a master order.
type OrderShipment struct {
	ID                   int64        `json:"id"`
	PublicID             string       `json:"public_id"`
	OrderID              int64        `json:"order_id"`
	OrganizationID       int64        `json:"organization_id"`
	BranchID             *int64       `json:"branch_id,omitempty"`
	ShipmentNumber       string       `json:"shipment_number"`
	Status               OrderStatus  `json:"status"`
	Subtotal             money.Amount `json:"subtotal"`
	ShippingFee          money.Amount `json:"shipping_fee"`
	TotalAmount          money.Amount `json:"total_amount"`
	TrackingNumber       string       `json:"tracking_number,omitempty"`
	CarrierName          string       `json:"carrier_name,omitempty"`
	DeliveryCode         string       `json:"delivery_code,omitempty"`
	DeliveryAttempts     int          `json:"delivery_attempts"`
	DeliveryLockedUntil  *time.Time   `json:"delivery_locked_until,omitempty"`
	DeliveryNotes        string       `json:"delivery_notes,omitempty"`
	CollectedAmountMinor int64        `json:"collected_amount_minor"`
	DeliveredByCourierAt *time.Time   `json:"delivered_by_courier_at,omitempty"`

	// CourierUserID is the delivery representative (مندوب) carrying this
	// parcel, and CourierAssignedAt is when they were handed it. The courier
	// portal orders a queue by that timestamp ascending, so the parcel that
	// has been waiting longest is the one at the top of the round.
	CourierUserID     *int64     `json:"courier_user_id,omitempty"`
	CourierAssignedAt *time.Time `json:"courier_assigned_at,omitempty"`
	CourierAssignedBy *int64     `json:"courier_assigned_by,omitempty"`

	Lines       []*OrderLine `json:"lines,omitempty"`
	ShippedAt   *time.Time   `json:"shipped_at,omitempty"`
	DeliveredAt *time.Time   `json:"delivered_at,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`

	// Enriched metadata for views
	OrderNumber                 string        `json:"order_number,omitempty"`
	VendorName                  i18n.Text     `json:"vendor_name,omitempty"`
	CustomerOrgName             i18n.Text     `json:"customer_org_name,omitempty"`
	CustomerBranchName          i18n.Text     `json:"customer_branch_name,omitempty"`
	CustomerBranchAddress       string        `json:"customer_branch_address,omitempty"`
	CustomerBranchPhone         string        `json:"customer_branch_phone,omitempty"`
	CustomerManagerName         string        `json:"customer_manager_name,omitempty"`
	CustomerBranchLatitude      *float64      `json:"customer_branch_latitude,omitempty"`
	CustomerBranchLongitude     *float64      `json:"customer_branch_longitude,omitempty"`
	CustomerBranchGoogleMapsURL string        `json:"customer_branch_google_maps_url,omitempty"`
	PaymentMethod               string        `json:"payment_method,omitempty"`
	PaymentStatus               PaymentStatus `json:"payment_status,omitempty"`
	Notes                       string        `json:"notes,omitempty"`
	CourierName                 string        `json:"courier_name,omitempty"`
	CourierPhone                string        `json:"courier_phone,omitempty"`
	IsNegotiation               bool          `json:"is_negotiation,omitempty"`
	NegotiationStatus           string        `json:"negotiation_status,omitempty"`
	NegotiationNotes            string        `json:"negotiation_notes,omitempty"`
}

// OrderLine is an immutable snapshot of a product variant purchased in an order.
type OrderLine struct {
	ID                     int64         `json:"id"`
	OrderID                int64         `json:"order_id"`
	ShipmentID             int64         `json:"shipment_id"`
	OrganizationID         int64         `json:"organization_id"`
	ProductID              *int64        `json:"product_id,omitempty"`
	ProductVariantID       *int64        `json:"product_variant_id,omitempty"`
	ProductName            i18n.Text     `json:"product_name"`
	VariantName            i18n.Text     `json:"variant_name,omitempty"`
	SKU                    string        `json:"sku,omitempty"`
	OfferProductID         *int64        `json:"offer_product_id,omitempty"` // offer_product sold under (063)
	UnitPrice              money.Amount  `json:"unit_price"`
	Quantity               int           `json:"quantity"`
	DiscountAmount         money.Amount  `json:"discount_amount"`
	TotalPrice             money.Amount  `json:"total_price"`
	CostPrice              *money.Amount `json:"cost_price,omitempty"`        // Optional snapshot of cost price at purchase
	CostDiscountPercentage float64       `json:"cost_discount_percentage"`    // Snapshot of cost discount %
	ListPrice              money.Amount  `json:"list_price,omitempty"`        // pre-discount strike price (063)
	OriginalPrice          money.Amount  `json:"original_price,omitempty"`    // legacy price snapshot (063)
	OriginalDiscount       money.Amount  `json:"original_discount,omitempty"` // legacy discount snapshot (063)
	IsNegotiated           bool          `json:"is_negotiated"`
	ProposedUnitPrice      money.Amount  `json:"proposed_unit_price,omitempty"`
	Rating                 *float64      `json:"rating,omitempty"` // per-line rating (Laravel adv_orders.rating parity)
	AvailableStock         int           `json:"available_stock,omitempty"`
	MinOrderQty            int           `json:"min_order_qty,omitempty"`
	MaxQtyPerOrder         int           `json:"max_qty_per_order,omitempty"`
	CreatedAt              time.Time     `json:"created_at"`
}

// VendorFinancialFilter holds optional criteria for filtering financial summaries.
type VendorFinancialFilter struct {
	DateFrom      string `json:"date_from,omitempty"`
	DateTo        string `json:"date_to,omitempty"`
	CustomerOrgID int64  `json:"customer_org_id,omitempty"`
}

// VendorFinancialSummary represents the unified financial and profit metrics for a vendor.
type VendorFinancialSummary struct {
	Period               string                  `json:"period"` // "month", "last_month", "year", "all", "custom"
	DateFrom             string                  `json:"date_from,omitempty"`
	DateTo               string                  `json:"date_to,omitempty"`
	CustomerOrgID        int64                   `json:"customer_org_id,omitempty"`
	GrossSales           money.Amount            `json:"gross_sales"`
	TotalDiscounts       money.Amount            `json:"total_discounts"`
	NetSales             money.Amount            `json:"net_sales"`
	COGS                 money.Amount            `json:"cogs"` // Total Cost of Goods Sold (discounted cost prices)
	PlatformFees         money.Amount            `json:"platform_fees"`
	NetProfit            money.Amount            `json:"net_profit"`
	ProfitMargin         float64                 `json:"profit_margin"`
	ProfitOnCost         float64                 `json:"profit_on_cost"` // Return on cost / markup % (NetProfit / COGS * 100)
	DeliveredOrdersCount int                     `json:"delivered_orders_count"`
	PendingOrdersTotal   money.Amount            `json:"pending_orders_total"`
	PendingOrdersCount   int                     `json:"pending_orders_count"`
	WalletBalance        money.Amount            `json:"wallet_balance"`
	Shipments            []*VendorShipmentProfit `json:"shipments,omitempty"`
	TopProducts          []*VendorProductProfit  `json:"top_products,omitempty"`
}

// VendorShipmentProfit gives financial breakdown for a single delivered shipment.
type VendorShipmentProfit struct {
	ShipmentID      int64        `json:"shipment_id"`
	ShipmentNumber  string       `json:"shipment_number"`
	OrderID         int64        `json:"order_id"`
	OrderNumber     string       `json:"order_number"`
	CustomerOrgName string       `json:"customer_org_name"`
	DeliveredAt     time.Time    `json:"delivered_at"`
	GrossSales      money.Amount `json:"gross_sales"`
	Discounts       money.Amount `json:"discounts"`
	NetSales        money.Amount `json:"net_sales"`
	COGS            money.Amount `json:"cogs"`
	PlatformFee     money.Amount `json:"platform_fee"`
	NetProfit       money.Amount `json:"net_profit"`
	ProfitMargin    float64      `json:"profit_margin"`
	ProfitOnCost    float64      `json:"profit_on_cost"` // Return on cost % (NetProfit / COGS * 100)
	PaymentStatus   string       `json:"payment_status"`
	LineItemsCount  int          `json:"line_items_count"`
}

// VendorProductProfit gives profitability analysis per catalog item.
type VendorProductProfit struct {
	ProductID              int64         `json:"product_id"`
	VariantID              int64         `json:"variant_id"`
	Name                   string        `json:"name"`
	SKU                    string        `json:"sku"`
	QuantitySold           int           `json:"quantity_sold"`
	PublicPrice            money.Amount  `json:"public_price"`     // Official list/retail price (سعر الجمهور)
	SellingDiscount        float64       `json:"selling_discount"` // Selling discount percentage relative to public price
	SellingPrice           money.Amount  `json:"selling_price"`    // Actual selling unit price
	CostPrice              *money.Amount `json:"cost_price,omitempty"`
	CostDiscountPercentage float64       `json:"cost_discount_percentage"` // Purchase/cost discount %
	DiscountedCost         money.Amount  `json:"discounted_cost"`          // Actual unit purchase cost
	TotalRevenue           money.Amount  `json:"total_revenue"`
	TotalCost              money.Amount  `json:"total_cost"`
	NetProfit              money.Amount  `json:"net_profit"`
	ProfitMargin           float64       `json:"profit_margin"`  // Profit margin % on revenue (NetProfit / Revenue * 100)
	ProfitOnCost           float64       `json:"profit_on_cost"` // Return on cost % (NetProfit / Cost * 100)
}

// OrderStatusHistory logs every transition of order/shipment status.
type OrderStatusHistory struct {
	ID              int64     `json:"id"`
	OrderID         int64     `json:"order_id"`
	ShipmentID      *int64    `json:"shipment_id,omitempty"`
	FromStatus      *string   `json:"from_status,omitempty"`
	ToStatus        string    `json:"to_status"`
	Notes           string    `json:"notes,omitempty"`
	ChangedByUserID *int64    `json:"changed_by_user_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// OrderLineEditItem represents an item modification when a customer edits a pending order.
type OrderLineEditItem struct {
	ID             int64        `json:"id"` // 0 if new line
	ProductName    string       `json:"product_name"`
	Quantity       int          `json:"quantity"`
	UnitPrice      money.Amount `json:"unit_price"`
	DiscountAmount money.Amount `json:"discount_amount"`
	IsDeleted      bool         `json:"is_deleted"`
}

// UpdateCustomerOrderInput represents the payload submitted by a pharmacist to edit their pending order.
type UpdateCustomerOrderInput struct {
	OrderID int64               `json:"order_id"`
	Lines   []OrderLineEditItem `json:"lines"`
	Notes   string              `json:"notes,omitempty"`
}
