package commerce

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockCommerceRepo struct {
	orders               map[int64]*Order
	shipments            map[int64][]*OrderShipment
	lines                map[int64][]*OrderLine
	history              map[int64][]*OrderStatusHistory
	wishlist             map[int64][]int64
	quotes               map[int64]*QuoteRequest
	purchaseRequests     map[int64]*PurchaseRequest
	purchaseRequestLines map[int64][]*PurchaseRequestLine
	nextID               int64
}

func newMockCommerceRepo() *mockCommerceRepo {
	return &mockCommerceRepo{
		orders:               map[int64]*Order{},
		shipments:            map[int64][]*OrderShipment{},
		history:              map[int64][]*OrderStatusHistory{},
		lines:                map[int64][]*OrderLine{},
		wishlist:             map[int64][]int64{},
		quotes:               map[int64]*QuoteRequest{},
		purchaseRequests:     map[int64]*PurchaseRequest{},
		purchaseRequestLines: map[int64][]*PurchaseRequestLine{},
		nextID:               1,
	}
}

func (m *mockCommerceRepo) GetOrCreateCart(_ context.Context, userID int64) (*Cart, error) {
	return &Cart{ID: 1, UserID: userID}, nil
}
func (m *mockCommerceRepo) GetCartWithItems(_ context.Context, cartID int64) (*Cart, error) {
	return &Cart{ID: cartID, UserID: 100}, nil
}
func (m *mockCommerceRepo) AddToCartItem(_ context.Context, cartID int64, item *CartItem) error {
	return nil
}
func (m *mockCommerceRepo) RemoveCartItemByID(_ context.Context, _, _ int64) error { return nil }

func (m *mockCommerceRepo) SetCartItemQuantityByID(_ context.Context, _, _ int64, _ int) error {
	return nil
}

func (m *mockCommerceRepo) RemoveCartItem(_ context.Context, cartID int64, variantID int64) error {
	return nil
}
func (m *mockCommerceRepo) ClearCart(_ context.Context, cartID int64) error { return nil }

func (m *mockCommerceRepo) CreateOrder(
	_ context.Context,
	order *Order,
	shipments []*OrderShipment,
	lines []*OrderLine,
) error {
	order.ID = m.nextID
	m.nextID++
	for i, s := range shipments {
		s.ID = m.nextID + int64(i)
		s.OrderID = order.ID
	}
	m.orders[order.ID] = order
	m.shipments[order.ID] = shipments
	m.lines[order.ID] = lines
	return nil
}

func (m *mockCommerceRepo) GetOrderByID(_ context.Context, id int64) (*Order, error) {
	o, ok := m.orders[id]
	if !ok {
		return nil, apperr.NotFound("order")
	}
	return o, nil
}

func (m *mockCommerceRepo) GetOrderByNumber(_ context.Context, number string) (*Order, error) {
	for _, o := range m.orders {
		if o.OrderNumber == number {
			return o, nil
		}
	}
	return nil, apperr.NotFound("order")
}

func (m *mockCommerceRepo) UpdateOrderStatus(
	_ context.Context,
	orderID int64,
	toStatus OrderStatus,
	history OrderStatusHistory,
) error {
	o, ok := m.orders[orderID]
	if !ok {
		return apperr.NotFound("order")
	}
	if !IsValidStatusTransition(o.Status, toStatus) {
		return apperr.Validation("status.invalid", "Invalid transition", nil)
	}
	o.Status = toStatus
	return nil
}

func (m *mockCommerceRepo) UpdateOrderPaymentStatus(_ context.Context, orderID int64, paymentStatus PaymentStatus) error {
	o, ok := m.orders[orderID]
	if !ok {
		return apperr.NotFound("order")
	}
	o.PaymentStatus = paymentStatus
	return nil
}

func (m *mockCommerceRepo) ListOrdersByCustomer(_ context.Context, customerID int64, limit, offset int) ([]*Order, error) {
	var list []*Order
	for _, o := range m.orders {
		if o.CustomerID == customerID {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockCommerceRepo) ListOrdersByCustomerWithTotal(ctx context.Context, customerID int64, limit, offset int) ([]*Order, int, error) {
	list, err := m.ListOrdersByCustomer(ctx, customerID, limit, offset)
	return list, len(list), err
}

func (m *mockCommerceRepo) ListShipmentsByVendor(_ context.Context, vendorOrgID int64, limit, offset int) ([]*OrderShipment, error) {
	var list []*OrderShipment
	for _, sList := range m.shipments {
		for _, s := range sList {
			if s.OrganizationID == vendorOrgID {
				list = append(list, s)
			}
		}
	}
	return list, nil
}

func (m *mockCommerceRepo) ListShipmentsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*OrderShipment, int, error) {
	list, err := m.ListShipmentsByVendor(ctx, vendorOrgID, limit, offset)
	return list, len(list), err
}

func (m *mockCommerceRepo) AddToWishlist(_ context.Context, userID int64, productID int64) error {
	m.wishlist[userID] = append(m.wishlist[userID], productID)
	return nil
}
func (m *mockCommerceRepo) RemoveFromWishlist(_ context.Context, userID int64, productID int64) error {
	var remaining []int64
	for _, id := range m.wishlist[userID] {
		if id != productID {
			remaining = append(remaining, id)
		}
	}
	m.wishlist[userID] = remaining
	return nil
}
func (m *mockCommerceRepo) ListWishlist(_ context.Context, userID int64) ([]*WishlistItem, error) {
	var items []*WishlistItem
	for _, pID := range m.wishlist[userID] {
		items = append(items, &WishlistItem{UserID: userID, ProductID: pID})
	}
	return items, nil
}

func (m *mockCommerceRepo) CreateQuoteRequest(_ context.Context, q *QuoteRequest) error {
	q.ID = m.nextID
	m.nextID++
	m.quotes[q.ID] = q
	return nil
}
func (m *mockCommerceRepo) GetQuoteRequestByID(_ context.Context, id int64) (*QuoteRequest, error) {
	q, ok := m.quotes[id]
	if !ok {
		return nil, apperr.NotFound("quote")
	}
	return q, nil
}
func (m *mockCommerceRepo) UpdateQuoteStatus(_ context.Context, id int64, status QuoteStatus, quotePrice money.Amount, supplierNotes string) error {
	q, ok := m.quotes[id]
	if !ok {
		return apperr.NotFound("quote")
	}
	q.Status = status
	q.QuoteUnitPrice = quotePrice
	q.SupplierNotes = supplierNotes
	return nil
}
func (m *mockCommerceRepo) ListQuoteRequestsByOrg(_ context.Context, orgID int64, isVendor bool, limit, offset int) ([]*QuoteRequest, error) {
	var list []*QuoteRequest
	for _, q := range m.quotes {
		if isVendor && q.OrganizationID == orgID {
			list = append(list, q)
		} else if !isVendor && q.CustomerOrgID == orgID {
			list = append(list, q)
		}
	}
	return list, nil
}

func (m *mockCommerceRepo) AdminSearchOrders(_ context.Context, query string, limit, offset int) ([]*Order, error) {
	var list []*Order
	for _, o := range m.orders {
		list = append(list, o)
	}
	return list, nil
}

func (m *mockCommerceRepo) AdminSearchOrdersWithTotal(_ context.Context, query, tab string, limit, offset int) ([]*Order, int, error) {
	var list []*Order
	for _, o := range m.orders {
		list = append(list, o)
	}
	return list, len(list), nil
}

func (m *mockCommerceRepo) AdminSearchOrdersFiltered(_ context.Context, filter AdminOrderFilter) ([]*Order, int, error) {
	var list []*Order
	for _, o := range m.orders {
		list = append(list, o)
	}
	return list, len(list), nil
}

func (m *mockCommerceRepo) AdminOrderStats(_ context.Context) (allCount, directCount, negotiationCount int, err error) {
	return len(m.orders), len(m.orders), 0, nil
}

func (m *mockCommerceRepo) AdminOrderKPIs(_ context.Context) (AdminOrderKPIs, error) {
	return AdminOrderKPIs{
		TotalOrders:  len(m.orders),
		DirectOrders: len(m.orders),
	}, nil
}

func (m *mockCommerceRepo) MonthSalesByVendor(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}

func (m *mockCommerceRepo) CountOrderLines(context.Context, []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (m *mockCommerceRepo) MonthSpendByCustomer(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}

func (m *mockCommerceRepo) CreatePurchaseRequest(_ context.Context, pr *PurchaseRequest, lines []*PurchaseRequestLine) error {
	pr.ID = m.nextID
	m.nextID++
	m.purchaseRequests[pr.ID] = pr
	for _, l := range lines {
		l.ID = m.nextID
		m.nextID++
		l.RequestID = pr.ID
	}
	m.purchaseRequestLines[pr.ID] = lines
	return nil
}

func (m *mockCommerceRepo) GetPurchaseRequestByID(_ context.Context, id int64) (*PurchaseRequest, error) {
	pr, ok := m.purchaseRequests[id]
	if !ok {
		return nil, apperr.NotFound("purchase_request")
	}
	pr.Lines = m.purchaseRequestLines[id]
	return pr, nil
}

func (m *mockCommerceRepo) GetPurchaseRequestByNumber(_ context.Context, number string) (*PurchaseRequest, error) {
	for _, pr := range m.purchaseRequests {
		if pr.RequestNumber == number {
			pr.Lines = m.purchaseRequestLines[pr.ID]
			return pr, nil
		}
	}
	return nil, apperr.NotFound("purchase_request")
}

func (m *mockCommerceRepo) ListPurchaseRequestsByCustomer(_ context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*PurchaseRequest, error) {
	var list []*PurchaseRequest
	for _, pr := range m.purchaseRequests {
		if pr.CustomerID == customerID || (orgID != nil && pr.OrganizationID != nil && *pr.OrganizationID == *orgID) {
			if status == "" || status == "all" || string(pr.Status) == status {
				pr.Lines = m.purchaseRequestLines[pr.ID]
				list = append(list, pr)
			}
		}
	}
	return list, nil
}

func (m *mockCommerceRepo) ListPurchaseRequestsByVendor(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*PurchaseRequest, error) {
	reqs, _, err := m.ListPurchaseRequestsByVendorWithTotal(ctx, vendorOrgID, status, limit, offset)
	return reqs, err
}

func (m *mockCommerceRepo) ListPurchaseRequestsByVendorWithTotal(_ context.Context, vendorOrgID int64, status string, limit, offset int) ([]*PurchaseRequest, int, error) {
	var list []*PurchaseRequest
	for _, pr := range m.purchaseRequests {
		if pr.VendorOrgID == vendorOrgID {
			if status == "" || status == "all" || string(pr.Status) == status {
				pr.Lines = m.purchaseRequestLines[pr.ID]
				list = append(list, pr)
			}
		}
	}
	total := len(list)
	return list, total, nil
}

func (m *mockCommerceRepo) CountPurchaseRequestsByCustomer(_ context.Context, customerID int64, orgID *int64) (map[string]int, error) {
	counts := make(map[string]int)
	total := 0
	for _, pr := range m.purchaseRequests {
		if pr.CustomerID == customerID || (orgID != nil && pr.OrganizationID != nil && *pr.OrganizationID == *orgID) {
			counts[string(pr.Status)]++
			total++
		}
	}
	counts["all"] = total
	return counts, nil
}

func (m *mockCommerceRepo) UpdatePurchaseRequestStatus(_ context.Context, id int64, status PurchaseRequestStatus, vendorNotes string, responderID *int64) error {
	pr, ok := m.purchaseRequests[id]
	if !ok {
		return apperr.NotFound("purchase_request")
	}
	pr.Status = status
	if vendorNotes != "" {
		pr.VendorNotes = vendorNotes
	}
	pr.RespondedBy = responderID
	return nil
}

func (m *mockCommerceRepo) UpdatePurchaseRequestLineOffer(_ context.Context, lineID int64, price money.Amount, discount float64, status string) error {
	for _, lines := range m.purchaseRequestLines {
		for _, l := range lines {
			if l.ID == lineID {
				l.OfferedPrice = price
				l.OfferedDiscount = discount
				l.Status = status
				return nil
			}
		}
	}
	return apperr.NotFound("purchase_request_line")
}

func (m *mockCommerceRepo) SetShipmentTracking(_ context.Context, _ int64, _, _ string) error {
	return nil
}

func (m *mockCommerceRepo) GetVendorFinancialSummary(_ context.Context, vendorOrgID int64, period string, filterOpt ...VendorFinancialFilter) (*VendorFinancialSummary, error) {
	return &VendorFinancialSummary{
		Period: period,
	}, nil
}

func (m *mockCommerceRepo) GetOfferDetailsForOrderLine(_ context.Context, _, _ int64) (*OrderLineOfferDetails, error) {
	return nil, nil
}

func (m *mockCommerceRepo) ListVendorNegotiationOrdersWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*Order, int, error) {
	return nil, 0, nil
}
