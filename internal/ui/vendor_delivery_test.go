package ui_test

// courierMockCommerceRepo stands in for the commerce repository across the
// delivery tests. It embeds commerce.Repository as a nil interface, so any
// method these tests do not stub panics loudly rather than returning a
// misleading zero value.

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type courierMockCommerceRepo struct {
	commerce.Repository
	shipment *commerce.OrderShipment
	// round, when set, is the caller's whole open queue. The single-shipment
	// field answers "is this parcel mine"; the route screen needs several
	// parcels at different places, which is a different question.
	round []*commerce.OrderShipment
}

func (m *courierMockCommerceRepo) GetOrCreateCart(_ context.Context, _ int64) (*commerce.Cart, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) GetCartWithItems(_ context.Context, _ int64) (*commerce.Cart, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) AddToCartItem(_ context.Context, _ int64, _ *commerce.CartItem) error {
	return nil
}
func (m *courierMockCommerceRepo) SetCartItemQuantity(_ context.Context, _, _ int64, _ int) error {
	return nil
}
func (m *courierMockCommerceRepo) RemoveCartItem(_ context.Context, _, _ int64) error { return nil }

func (m *courierMockCommerceRepo) RemoveCartItemByID(_ context.Context, _, _ int64) error {
	return nil
}

func (m *courierMockCommerceRepo) SetCartItemQuantityByID(_ context.Context, _, _ int64, _ int) error {
	return nil
}
func (m *courierMockCommerceRepo) ClearCart(_ context.Context, _ int64) error { return nil }
func (m *courierMockCommerceRepo) CreateOrder(_ context.Context, _ *commerce.Order, _ []*commerce.OrderShipment, _ []*commerce.OrderLine) error {
	return nil
}
func (m *courierMockCommerceRepo) GetOrderByID(_ context.Context, id int64) (*commerce.Order, error) {
	return &commerce.Order{ID: id, OrderNumber: "ORD-999", CustomerID: 10}, nil
}
func (m *courierMockCommerceRepo) GetOrderByNumber(_ context.Context, _ string) (*commerce.Order, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) UpdateOrderStatus(_ context.Context, _ int64, _ commerce.OrderStatus, _ commerce.OrderStatusHistory) error {
	return nil
}
func (m *courierMockCommerceRepo) UpdateOrderPaymentStatus(_ context.Context, _ int64, _ commerce.PaymentStatus) error {
	return nil
}
func (m *courierMockCommerceRepo) UpdateCustomerPendingOrder(_ context.Context, _ *commerce.Order, _ []commerce.OrderLineEditItem, _ int64) (*commerce.Order, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) ListOrdersByCustomer(_ context.Context, _ int64, _, _ int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) ListOrdersByCustomerWithTotal(_ context.Context, _ int64, _, _ int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *courierMockCommerceRepo) CountOrders(_ context.Context) (int, error) { return 0, nil }
func (m *courierMockCommerceRepo) CountVendorShipmentsByStatus(_ context.Context, _ int64, _ []string) (int, error) {
	return 0, nil
}
func (m *courierMockCommerceRepo) MonthSalesByVendor(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *courierMockCommerceRepo) CountOrderLines(context.Context, []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (m *courierMockCommerceRepo) MonthSpendByCustomer(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *courierMockCommerceRepo) ListShipmentsByVendor(_ context.Context, _ int64, _, _ int) ([]*commerce.OrderShipment, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) ListShipmentsByVendorWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*commerce.OrderShipment, int, error) {
	return nil, 0, nil
}
func (m *courierMockCommerceRepo) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == id {
		c := *m.shipment
		return &c, nil
	}
	return nil, apperr.NotFound("shipment")
}
func (m *courierMockCommerceRepo) UpdateShipmentStatus(_ context.Context, id int64, _, to commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	if m.shipment != nil && m.shipment.ID == id {
		m.shipment.Status = to
		if history.Notes != "" {
			m.shipment.DeliveryNotes = history.Notes
		}
		return nil
	}
	return nil
}
func (m *courierMockCommerceRepo) SetShipmentTracking(_ context.Context, id int64, carrier, tracking string) error {
	if m.shipment != nil && m.shipment.ID == id {
		m.shipment.CarrierName = carrier
		m.shipment.TrackingNumber = tracking
		return nil
	}
	return nil
}
func (m *courierMockCommerceRepo) ListOrderHistory(_ context.Context, _ int64) ([]*commerce.OrderStatusHistory, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) RateOrder(_ context.Context, _, _ int64, _ float64, _ string) error {
	return nil
}
func (m *courierMockCommerceRepo) AddToWishlist(_ context.Context, _, _ int64) error      { return nil }
func (m *courierMockCommerceRepo) RemoveFromWishlist(_ context.Context, _, _ int64) error { return nil }
func (m *courierMockCommerceRepo) ListWishlist(_ context.Context, _ int64) ([]*commerce.WishlistItem, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) CreateQuoteRequest(_ context.Context, _ *commerce.QuoteRequest) error {
	return nil
}
func (m *courierMockCommerceRepo) GetQuoteRequestByID(_ context.Context, _ int64) (*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) UpdateQuoteStatus(_ context.Context, _ int64, _ commerce.QuoteStatus, _ money.Amount, _ string) error {
	return nil
}
func (m *courierMockCommerceRepo) ListQuoteRequestsByOrg(_ context.Context, _ int64, _ bool, _, _ int) ([]*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) CreatePurchaseRequest(_ context.Context, _ *commerce.PurchaseRequest, _ []*commerce.PurchaseRequestLine) error {
	return nil
}
func (m *courierMockCommerceRepo) GetPurchaseRequestByID(_ context.Context, _ int64) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) GetPurchaseRequestByNumber(_ context.Context, _ string) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) ListPurchaseRequestsByCustomer(_ context.Context, _ int64, _ *int64, _ string, _, _ int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) ListPurchaseRequestsByVendor(_ context.Context, _ int64, _ string, _, _ int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) CountPurchaseRequestsByCustomer(_ context.Context, _ int64, _ *int64) (map[string]int, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) UpdatePurchaseRequestStatus(_ context.Context, _ int64, _ commerce.PurchaseRequestStatus, _ string, _ *int64) error {
	return nil
}
func (m *courierMockCommerceRepo) UpdatePurchaseRequestLineOffer(_ context.Context, _ int64, _ money.Amount, _ float64, _ string) error {
	return nil
}
func (m *courierMockCommerceRepo) AdminSearchOrders(_ context.Context, _ string, _, _ int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) AdminSearchOrdersWithTotal(_ context.Context, _, _ string, _, _ int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *courierMockCommerceRepo) AdminSearchOrdersFiltered(_ context.Context, _ commerce.AdminOrderFilter) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *courierMockCommerceRepo) AdminOrderStats(_ context.Context) (int, int, int, error) {
	return 0, 0, 0, nil
}
func (m *courierMockCommerceRepo) AdminOrderKPIs(_ context.Context) (commerce.AdminOrderKPIs, error) {
	return commerce.AdminOrderKPIs{}, nil
}
func (m *courierMockCommerceRepo) AcceptNegotiation(_ context.Context, _, _ int64) error { return nil }
func (m *courierMockCommerceRepo) RejectNegotiation(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}
func (m *courierMockCommerceRepo) ListVendorNegotiationOrdersWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *courierMockCommerceRepo) GetVendorFinancialSummary(_ context.Context, _ int64, period string, filterOpt ...commerce.VendorFinancialFilter) (*commerce.VendorFinancialSummary, error) {
	return &commerce.VendorFinancialSummary{Period: period}, nil
}

// --- The dispatch board ---

func (m *courierMockCommerceRepo) GetVendorShipment(_ context.Context, shipmentID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == shipmentID && m.shipment.OrganizationID == vendorOrgID {
		c := *m.shipment
		return &c, nil
	}
	return nil, apperr.NotFound("shipment")
}

func (m *courierMockCommerceRepo) AssignShipmentCourier(_ context.Context, shipmentID, vendorOrgID int64, courierUserID *int64, assignedBy int64) error {
	if m.shipment == nil || m.shipment.ID != shipmentID || m.shipment.OrganizationID != vendorOrgID {
		return apperr.NotFound("shipment")
	}
	m.shipment.CourierUserID = courierUserID
	if courierUserID == nil {
		m.shipment.CourierAssignedAt = nil
		m.shipment.CourierAssignedBy = nil
		return nil
	}
	now := time.Now()
	m.shipment.CourierAssignedAt = &now
	m.shipment.CourierAssignedBy = &assignedBy
	return nil
}

// ListCourierQueue reproduces the four predicates the SQL applies, so a test
// that asks for the wrong queue gets the wrong answer here too.
func (m *courierMockCommerceRepo) ListCourierQueue(_ context.Context, f commerce.CourierQueueFilter) ([]*commerce.OrderShipment, int, error) {
	if len(m.round) > 0 && f.Queue == commerce.CourierQueueMine && f.CourierUserID == assignedCourierID {
		return m.round, len(m.round), nil
	}
	if m.shipment == nil || m.shipment.OrganizationID != f.VendorOrgID {
		return nil, 0, nil
	}
	mine := m.shipment.IsAssignedTo(f.CourierUserID)
	closed := m.shipment.IsClosed()
	var match bool
	switch f.Queue {
	case commerce.CourierQueueMine:
		match = mine && !closed
	case commerce.CourierQueueCompleted:
		match = mine && closed && (m.shipment.Status == commerce.StatusDelivered || m.shipment.Status == commerce.StatusCompleted)
	case commerce.CourierQueueFailed:
		match = mine && closed && (m.shipment.Status == commerce.StatusFailed || m.shipment.Status == commerce.StatusReturned || m.shipment.Status == commerce.StatusCancelled)
	case commerce.CourierQueueUnassigned:
		match = m.shipment.CourierUserID == nil && !closed
	case commerce.CourierQueueAll:
		match = !closed
	}
	if !match {
		return nil, 0, nil
	}
	c := *m.shipment
	return []*commerce.OrderShipment{&c}, 1, nil
}

func (m *courierMockCommerceRepo) CourierQueueCounts(_ context.Context, _, courierUserID int64) (commerce.CourierQueueCounts, error) {
	var c commerce.CourierQueueCounts
	if len(m.round) > 0 && courierUserID == assignedCourierID {
		c.Mine = len(m.round)
		c.All = len(m.round)
		return c, nil
	}
	if m.shipment == nil {
		return c, nil
	}
	if m.shipment.IsAssignedTo(courierUserID) {
		if m.shipment.IsClosed() {
			if m.shipment.Status == commerce.StatusFailed || m.shipment.Status == commerce.StatusReturned || m.shipment.Status == commerce.StatusCancelled {
				c.Failed = 1
			} else {
				c.Completed = 1
			}
		} else {
			c.Mine = 1
		}
	}
	if !m.shipment.IsClosed() {
		c.All = 1
		if m.shipment.CourierUserID == nil {
			c.Unassigned = 1
		}
	}
	return c, nil
}

func (m *courierMockCommerceRepo) ListCourierWorkload(_ context.Context, _ int64) ([]*commerce.CourierWorkload, error) {
	return nil, nil
}

func (m *courierMockCommerceRepo) VerifyAndCompleteDelivery(
	_ context.Context,
	shipmentID int64,
	deliveryCode string,
	notes string,
	collectedAmountMinor int64,
) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == shipmentID {
		if m.shipment.DeliveryCode != "" && m.shipment.DeliveryCode != deliveryCode {
			return nil, apperr.Validation("delivery.invalid_code", "كود تأكيد الاستلام غير صحيح.", nil)
		}
		m.shipment.Status = commerce.StatusDelivered
		now := time.Now()
		m.shipment.DeliveredAt = &now
		m.shipment.DeliveredByCourierAt = &now
		m.shipment.DeliveryNotes = notes
		m.shipment.CollectedAmountMinor = collectedAmountMinor
		c := *m.shipment
		return &c, nil
	}
	return nil, apperr.NotFound("shipment")
}
