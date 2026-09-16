package commerce

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// UpdateCustomerPendingOrder allows a customer/pharmacist to edit items and quantities of an order
// strictly before it is accepted/confirmed or processed by the vendor.
func (s *Service) UpdateCustomerPendingOrder(ctx context.Context, actor authctx.Actor, input UpdateCustomerOrderInput) (*Order, error) {
	if input.OrderID <= 0 {
		return nil, apperr.Validation("order.invalid_id", i18n.TDefault("w4_mod.s_356_356"), nil)
	}

	order, err := s.repo.GetOrderByID(ctx, input.OrderID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, apperr.NotFound("order")
	}

	// Verify authorization: must belong to the customer
	isOwner := actor.UserID == order.CustomerID || (order.OrganizationID != nil && *order.OrganizationID == actor.OrganizationID) || (order.OrganizationID != nil && *order.OrganizationID == actor.OrgID)
	if !isOwner && !actor.IsPlatformAdmin() {
		return nil, apperr.Forbidden("order.unauthorized", i18n.TDefault("w4_mod.s_357_357"))
	}

	// Check lifecycle state: strictly only StatusPending can be edited
	if order.Status != StatusPending {
		return nil, apperr.Forbidden("order.locked", i18n.TDefault("w4_mod.w4str_144_144")+string(order.Status)+")")
	}

	return s.repo.UpdateCustomerPendingOrder(ctx, order, input.Lines, actor.UserID)
}

// CountOrders returns the platform-wide order total, for admin dashboards.
func (s *Service) CountOrders(ctx context.Context) (int, error) {
	return s.repo.CountOrders(ctx)
}

// ListCustomerOrders retrieves paginated orders for a customer.
func (s *Service) ListCustomerOrders(ctx context.Context, customerID int64, limit, offset int) ([]*Order, error) {
	return s.repo.ListOrdersByCustomer(ctx, customerID, limit, offset)
}

// ListCustomerOrdersWithTotal retrieves paginated orders for a customer with total count.
func (s *Service) ListCustomerOrdersWithTotal(ctx context.Context, customerID int64, limit, offset int) ([]*Order, int, error) {
	return s.repo.ListOrdersByCustomerWithTotal(ctx, customerID, limit, offset)
}

// ListVendorShipments retrieves paginated shipments for a vendor.
func (s *Service) ListVendorShipments(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*OrderShipment, error) {
	return s.repo.ListShipmentsByVendor(ctx, vendorOrgID, limit, offset)
}

// ListVendorShipmentsWithTotal retrieves paginated shipments for a vendor with status filter and total count.
func (s *Service) ListVendorShipmentsWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*OrderShipment, int, error) {
	return s.repo.ListShipmentsByVendorWithTotal(ctx, vendorOrgID, status, limit, offset)
}

// GetVendorShipment returns one shipment scoped to the vendor organization.
func (s *Service) GetVendorShipment(ctx context.Context, shipmentID, vendorOrgID int64) (*OrderShipment, error) {
	return s.repo.GetVendorShipment(ctx, shipmentID, vendorOrgID)
}

// GetVendorShipmentOrByOrderID tries to find the vendor's shipment by its shipment ID,
// and if not found, falls back to finding it by the parent order ID.
func (s *Service) GetVendorShipmentOrByOrderID(ctx context.Context, id, vendorOrgID int64) (*OrderShipment, error) {
	sh, err := s.repo.GetVendorShipment(ctx, id, vendorOrgID)
	if err == nil && sh != nil {
		return sh, nil
	}
	return s.repo.GetVendorShipmentByOrderID(ctx, id, vendorOrgID)
}

// ListOrderHistory returns the chronological status history for an order/shipment.
func (s *Service) ListOrderHistory(ctx context.Context, orderID int64) ([]*OrderStatusHistory, error) {
	return s.repo.ListOrderHistory(ctx, orderID)
}

// MonthSalesByVendor returns the vendor's sales total for the current month.
func (s *Service) MonthSalesByVendor(ctx context.Context, vendorOrgID int64) (money.Amount, error) {
	return s.repo.MonthSalesByVendor(ctx, vendorOrgID)
}

// GetVendorFinancialSummary computes the comprehensive financial and net profit summary for a vendor.
func (s *Service) GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string, filterOpt ...VendorFinancialFilter) (*VendorFinancialSummary, error) {
	return s.repo.GetVendorFinancialSummary(ctx, vendorOrgID, period, filterOpt...)
}

// MonthSpendByCustomer returns the buyer's spend total for the current month.
func (s *Service) MonthSpendByCustomer(ctx context.Context, customerID int64) (money.Amount, error) {
	return s.repo.MonthSpendByCustomer(ctx, customerID)
}

// CreateQuoteRequest creates a new price inquiry from buyer to supplier.
func (s *Service) CreateQuoteRequest(ctx context.Context, q *QuoteRequest) (*QuoteRequest, error) {
	if q.OrganizationID <= 0 || q.CustomerOrgID <= 0 {
		return nil, apperr.Validation("quote.orgs_required", "Vendor and customer organization IDs are required.", nil)
	}
	if q.RequestedQuantity <= 0 {
		return nil, apperr.Validation("quote.qty_invalid", "Requested quantity must be positive.", nil)
	}
	q.Status = QuotePending
	if err := s.repo.CreateQuoteRequest(ctx, q); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "quote requested", "quote_id", q.ID, "vendor_org", q.OrganizationID, "customer_org", q.CustomerOrgID)
	return q, nil
}

// RespondToQuote allows vendor to provide quote unit price or reject.
// GetQuoteRequest loads one quote request.
func (s *Service) GetQuoteRequest(ctx context.Context, id int64) (*QuoteRequest, error) {
	return s.repo.GetQuoteRequestByID(ctx, id)
}

func (s *Service) RespondToQuote(ctx context.Context, quoteID int64, status QuoteStatus, price money.Amount, notes string) error {
	return s.repo.UpdateQuoteStatus(ctx, quoteID, status, price, notes)
}

// ListQuoteRequests lists quotes for an organization.
func (s *Service) ListQuoteRequests(ctx context.Context, orgID int64, isVendor bool, limit, offset int) ([]*QuoteRequest, error) {
	return s.repo.ListQuoteRequestsByOrg(ctx, orgID, isVendor, limit, offset)
}

// SetShipmentTracking records the carrier and tracking number for a shipment.
func (s *Service) SetShipmentTracking(ctx context.Context, id int64, carrier, tracking string) error {
	return s.repo.SetShipmentTracking(ctx, id, carrier, tracking)
}

// CountVendorShipmentsByStatus returns a vendor's shipment total in the given
// statuses, for dashboards that need a figure rather than a page.
func (s *Service) CountVendorShipmentsByStatus(ctx context.Context, orgID int64, statuses []string) (int, error) {
	return s.repo.CountVendorShipmentsByStatus(ctx, orgID, statuses)
}

// CreatePurchaseRequest creates a multi-line purchase request for a customer (Plan V5 Phase 3 §3.1).
func (s *Service) CreatePurchaseRequest(ctx context.Context, pr *PurchaseRequest, lines []*PurchaseRequestLine) (*PurchaseRequest, error) {
	if pr.CustomerID <= 0 || pr.VendorOrgID <= 0 {
		return nil, apperr.Validation("purchase_request.invalid_parties", "Customer and target supplier are required.", nil)
	}
	if len(lines) == 0 {
		return nil, apperr.Validation("purchase_request.empty_lines", "At least one product item is required.", nil)
	}

	for _, l := range lines {
		if l.ProductName == "" {
			return nil, apperr.Validation("purchase_request.invalid_line", "Product name is required for all line items.", nil)
		}
		if l.Quantity <= 0 {
			return nil, apperr.Validation("purchase_request.invalid_quantity", "Quantity must be greater than zero.", nil)
		}
	}

	if pr.Status == "" {
		pr.Status = PurchaseRequestPending
	}
	pr.TotalItems = len(lines)
	if pr.RequestNumber == "" {
		pr.RequestNumber = GeneratePurchaseRequestNumber(time.Now().UTC(), time.Now().UnixNano())
	}
	if err := s.repo.CreatePurchaseRequest(ctx, pr, lines); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "purchase request created", "request_id", pr.ID, "request_number", pr.RequestNumber, "vendor_org", pr.VendorOrgID)
	return pr, nil
}

// GetPurchaseRequest retrieves a purchase request with its lines.
func (s *Service) GetPurchaseRequest(ctx context.Context, id int64) (*PurchaseRequest, error) {
	return s.repo.GetPurchaseRequestByID(ctx, id)
}

// GetPurchaseRequestByNumber retrieves a purchase request by request number.
func (s *Service) GetPurchaseRequestByNumber(ctx context.Context, number string) (*PurchaseRequest, error) {
	return s.repo.GetPurchaseRequestByNumber(ctx, number)
}

// ListCustomerPurchaseRequests lists purchase requests placed by a customer.
func (s *Service) ListCustomerPurchaseRequests(ctx context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*PurchaseRequest, error) {
	if limit <= 0 {
		limit = 25
	}
	return s.repo.ListPurchaseRequestsByCustomer(ctx, customerID, orgID, status, limit, offset)
}

// ListVendorPurchaseRequests lists incoming purchase requests for a supplier.
func (s *Service) ListVendorPurchaseRequests(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*PurchaseRequest, error) {
	if limit <= 0 {
		limit = 25
	}
	return s.repo.ListPurchaseRequestsByVendor(ctx, vendorOrgID, status, limit, offset)
}

// ListVendorPurchaseRequestsWithTotal lists incoming purchase requests for a supplier with total count.
func (s *Service) ListVendorPurchaseRequestsWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*PurchaseRequest, int, error) {
	if limit <= 0 {
		limit = 25
	}
	return s.repo.ListPurchaseRequestsByVendorWithTotal(ctx, vendorOrgID, status, limit, offset)
}

// CountCustomerPurchaseRequests returns count statistics by status.
func (s *Service) CountCustomerPurchaseRequests(ctx context.Context, customerID int64, orgID *int64) (map[string]int, error) {
	return s.repo.CountPurchaseRequestsByCustomer(ctx, customerID, orgID)
}

// RespondPurchaseRequest allows a vendor to accept, modify, or reject a purchase request.
func (s *Service) RespondPurchaseRequest(ctx context.Context, id int64, status PurchaseRequestStatus, vendorNotes string, responderID *int64) error {
	return s.repo.UpdatePurchaseRequestStatus(ctx, id, status, vendorNotes, responderID)
}

// UpdatePurchaseRequestLineOffer allows vendor to set offered price/discount on a line item.
func (s *Service) UpdatePurchaseRequestLineOffer(ctx context.Context, lineID int64, price money.Amount, discount float64, status string) error {
	return s.repo.UpdatePurchaseRequestLineOffer(ctx, lineID, price, discount, status)
}

// AcceptNegotiation allows vendor or admin to accept a price negotiation on an order.
func (s *Service) AcceptNegotiation(ctx context.Context, orderID int64, actorID int64) error {
	if orderID <= 0 {
		return apperr.Validation("order.invalid_id", "Valid order ID is required.", nil)
	}
	return s.repo.AcceptNegotiation(ctx, orderID, actorID)
}

// RejectNegotiation allows vendor or admin to reject a price negotiation on an order.
func (s *Service) RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error {
	if orderID <= 0 {
		return apperr.Validation("order.invalid_id", "Valid order ID is required.", nil)
	}
	return s.repo.RejectNegotiation(ctx, orderID, reason, actorID)
}

// GetOfferDetailsForOrderLine returns the offer details and bundled items for an order line.
func (s *Service) GetOfferDetailsForOrderLine(ctx context.Context, orderID, lineID int64) (*OrderLineOfferDetails, error) {
	if orderID <= 0 || lineID <= 0 {
		return nil, apperr.Validation("order.invalid_id", "Valid order ID and line ID are required.", nil)
	}
	return s.repo.GetOfferDetailsForOrderLine(ctx, orderID, lineID)
}

// ListVendorNegotiationOrdersWithTotal retrieves orders with price negotiation for a vendor organization.
func (s *Service) ListVendorNegotiationOrdersWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*Order, int, error) {
	if vendorOrgID <= 0 {
		return nil, 0, apperr.Validation("vendor.invalid_id", "Valid vendor organization ID is required.", nil)
	}
	return s.repo.ListVendorNegotiationOrdersWithTotal(ctx, vendorOrgID, status, limit, offset)
}

// GetOrder retrieves an order by primary key.
func (s *Service) CountOrderLines(ctx context.Context, orderIDs []int64) (map[int64]int, error) {
	if len(orderIDs) == 0 {
		return map[int64]int{}, nil
	}
	return s.repo.CountOrderLines(ctx, orderIDs)
}

// GetOrder retrieves an order with its shipments and lines.
func (s *Service) GetOrder(ctx context.Context, id int64) (*Order, error) {
	return s.repo.GetOrderByID(ctx, id)
}

// GetOrderByNumber retrieves an order by public order number.
func (s *Service) GetOrderByNumber(ctx context.Context, number string) (*Order, error) {
	return s.repo.GetOrderByNumber(ctx, number)
}

// TransitionOrderStatus validates and applies an order state change.
func (s *Service) TransitionOrderStatus(
	ctx context.Context,
	orderID int64,
	newStatus OrderStatus,
	changedByUserID *int64,
	notes string,
) error {
	history := OrderStatusHistory{
		OrderID:         orderID,
		ToStatus:        string(newStatus),
		Notes:           notes,
		ChangedByUserID: changedByUserID,
	}
	return s.repo.UpdateOrderStatus(ctx, orderID, newStatus, history)
}

// CancelOrder transitions an order to cancelled status.
func (s *Service) CancelOrder(ctx context.Context, orderID int64, changedByUserID *int64, reason string) error {
	return s.TransitionOrderStatus(ctx, orderID, StatusCancelled, changedByUserID, reason)
}

// UpdateOrderPaymentStatus updates the payment status of an order.
func (s *Service) UpdateOrderPaymentStatus(ctx context.Context, orderID int64, paymentStatus PaymentStatus) error {
	return s.repo.UpdateOrderPaymentStatus(ctx, orderID, paymentStatus)
}

