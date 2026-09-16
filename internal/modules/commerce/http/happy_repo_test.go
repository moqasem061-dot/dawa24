package http_test

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type happyRepo struct{}

func (happyRepo) GetOrCreateCart(ctx context.Context, userID int64) (*commerce.Cart, error) {
	return &commerce.Cart{ID: 1, UserID: userID}, nil
}
func (happyRepo) GetCartWithItems(ctx context.Context, cartID int64) (*commerce.Cart, error) {
	return &commerce.Cart{ID: cartID, UserID: 1, Items: []*commerce.CartItem{{ID: 1, CartID: cartID, ProductID: 1, ProductVariantID: 1, Quantity: 2, UnitPrice: money.MustParse("50.00")}}}, nil
}
func (happyRepo) AddToCartItem(ctx context.Context, cartID int64, item *commerce.CartItem) error {
	return nil
}
func (happyRepo) SetCartItemQuantity(ctx context.Context, cartID int64, variantID int64, quantity int) error {
	return nil
}
func (happyRepo) RemoveCartItemByID(ctx context.Context, cartID, itemID int64) error { return nil }

func (happyRepo) SetCartItemQuantityByID(ctx context.Context, cartID, itemID int64, qty int) error {
	return nil
}

func (happyRepo) RemoveCartItem(ctx context.Context, cartID int64, variantID int64) error {
	return nil
}
func (happyRepo) ClearCart(ctx context.Context, cartID int64) error {
	return nil
}
func (happyRepo) CreateOrder(ctx context.Context, order *commerce.Order, shipments []*commerce.OrderShipment, lines []*commerce.OrderLine) error {
	order.ID = 1
	order.OrderNumber = "ORD-2026-0001"
	return nil
}
func (happyRepo) GetOrderByID(ctx context.Context, id int64) (*commerce.Order, error) {
	return &commerce.Order{ID: id, CustomerID: 1, OrderNumber: "ORD-1", Status: commerce.StatusDelivered, TotalAmount: money.MustParse("100.00")}, nil
}
func (happyRepo) GetOrderByNumber(ctx context.Context, number string) (*commerce.Order, error) {
	return &commerce.Order{ID: 1, CustomerID: 1, OrderNumber: number, Status: commerce.StatusDelivered}, nil
}
func (happyRepo) UpdateOrderStatus(ctx context.Context, orderID int64, toStatus commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	return nil
}
func (happyRepo) UpdateOrderPaymentStatus(ctx context.Context, orderID int64, paymentStatus commerce.PaymentStatus) error {
	return nil
}
func (happyRepo) ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, error) {
	return []*commerce.Order{{ID: 1, CustomerID: customerID, OrderNumber: "ORD-1"}}, nil
}
func (happyRepo) ListOrdersByCustomerWithTotal(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, int, error) {
	return []*commerce.Order{{ID: 1, CustomerID: customerID, OrderNumber: "ORD-1"}}, 1, nil
}
func (happyRepo) ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	return []*commerce.OrderShipment{{ID: 1, OrganizationID: vendorOrgID, ShipmentNumber: "SH-1"}}, nil
}
func (happyRepo) ListShipmentsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.OrderShipment, int, error) {
	return []*commerce.OrderShipment{{ID: 1, OrganizationID: vendorOrgID, ShipmentNumber: "SH-1"}}, 1, nil
}
func (happyRepo) GetShipmentByID(ctx context.Context, id int64) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{ID: id, OrganizationID: 1, Status: commerce.StatusPending}, nil
}
func (happyRepo) UpdateShipmentStatus(ctx context.Context, id int64, from, to commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	return nil
}
func (happyRepo) VerifyAndCompleteDelivery(ctx context.Context, shipmentID int64, deliveryCode string, notes string, collectedAmountMinor int64) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{ID: shipmentID, Status: commerce.StatusDelivered}, nil
}
func (happyRepo) ListOrderHistory(ctx context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	return []*commerce.OrderStatusHistory{{ID: 1, OrderID: orderID, ToStatus: string(commerce.StatusPending)}}, nil
}
func (happyRepo) RateOrder(ctx context.Context, orderID int64, customerID int64, rating float64, review string) error {
	return nil
}
func (happyRepo) AddToWishlist(ctx context.Context, userID int64, productID int64) error {
	return nil
}
func (happyRepo) RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error {
	return nil
}
func (happyRepo) ListWishlist(ctx context.Context, userID int64) ([]*commerce.WishlistItem, error) {
	return []*commerce.WishlistItem{{ID: 1, UserID: userID, ProductID: 1}}, nil
}
func (happyRepo) CreateQuoteRequest(ctx context.Context, qr *commerce.QuoteRequest) error {
	qr.ID = 1
	return nil
}
func (happyRepo) GetQuoteRequestByID(ctx context.Context, id int64) (*commerce.QuoteRequest, error) {
	return &commerce.QuoteRequest{ID: id, OrganizationID: 1, CustomerOrgID: 2, Status: commerce.QuotePending}, nil
}
func (happyRepo) UpdateQuoteStatus(ctx context.Context, id int64, status commerce.QuoteStatus, quotePrice money.Amount, supplierNotes string) error {
	return nil
}
func (happyRepo) ListQuoteRequestsByOrg(ctx context.Context, orgID int64, asSupplier bool, limit, offset int) ([]*commerce.QuoteRequest, error) {
	return []*commerce.QuoteRequest{{ID: 1, OrganizationID: orgID, Status: commerce.QuotePending}}, nil
}
func (happyRepo) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	return []*commerce.Order{{ID: 1, CustomerID: 1, OrderNumber: "ORD-1"}}, nil
}
func (happyRepo) AdminSearchOrdersWithTotal(ctx context.Context, query, tab string, limit, offset int) ([]*commerce.Order, int, error) {
	return []*commerce.Order{{ID: 1, CustomerID: 1, OrderNumber: "ORD-1"}}, 1, nil
}
func (happyRepo) AdminSearchOrdersFiltered(ctx context.Context, filter commerce.AdminOrderFilter) ([]*commerce.Order, int, error) {
	return []*commerce.Order{{ID: 1, CustomerID: 1, OrderNumber: "ORD-1"}}, 1, nil
}
func (happyRepo) AdminOrderStats(ctx context.Context) (int, int, int, error) {
	return 1, 1, 0, nil
}
func (happyRepo) AdminOrderKPIs(ctx context.Context) (commerce.AdminOrderKPIs, error) {
	return commerce.AdminOrderKPIs{TotalOrders: 1, DirectOrders: 1}, nil
}
func (happyRepo) CreatePurchaseRequest(ctx context.Context, pr *commerce.PurchaseRequest, lines []*commerce.PurchaseRequestLine) error {
	pr.ID = 1
	pr.RequestNumber = "PR-2026-0001"
	return nil
}
func (happyRepo) GetPurchaseRequestByID(ctx context.Context, id int64) (*commerce.PurchaseRequest, error) {
	return &commerce.PurchaseRequest{ID: id, RequestNumber: "PR-1", Status: commerce.PurchaseRequestPending}, nil
}
func (happyRepo) GetPurchaseRequestByNumber(ctx context.Context, number string) (*commerce.PurchaseRequest, error) {
	return &commerce.PurchaseRequest{ID: 1, RequestNumber: number, Status: commerce.PurchaseRequestPending}, nil
}
func (happyRepo) ListPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	return []*commerce.PurchaseRequest{{ID: 1, RequestNumber: "PR-1", Status: commerce.PurchaseRequestPending}}, nil
}
func (happyRepo) ListPurchaseRequestsByVendor(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	return []*commerce.PurchaseRequest{{ID: 1, RequestNumber: "PR-1", Status: commerce.PurchaseRequestPending}}, nil
}
func (happyRepo) ListPurchaseRequestsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, int, error) {
	return []*commerce.PurchaseRequest{{ID: 1, RequestNumber: "PR-1", Status: commerce.PurchaseRequestPending}}, 1, nil
}
func (happyRepo) CountPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64) (map[string]int, error) {
	return map[string]int{"all": 1, "pending": 1}, nil
}
func (happyRepo) UpdatePurchaseRequestStatus(ctx context.Context, id int64, status commerce.PurchaseRequestStatus, vendorNotes string, responderID *int64) error {
	return nil
}
func (happyRepo) UpdatePurchaseRequestLineOffer(ctx context.Context, lineID int64, price money.Amount, discount float64, status string) error {
	return nil
}
func (happyRepo) AcceptNegotiation(ctx context.Context, orderID int64, actorID int64) error {
	return nil
}
func (happyRepo) RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error {
	return nil
}
func (happyRepo) ListVendorNegotiationOrdersWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (happyRepo) UpdateCustomerPendingOrder(ctx context.Context, order *commerce.Order, lines []commerce.OrderLineEditItem, changedByUserID int64) (*commerce.Order, error) {
	return order, nil
}
func (happyRepo) GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string, filterOpt ...commerce.VendorFinancialFilter) (*commerce.VendorFinancialSummary, error) {
	return &commerce.VendorFinancialSummary{Period: period}, nil
}
func (happyRepo) GetOfferDetailsForOrderLine(ctx context.Context, orderID, lineID int64) (*commerce.OrderLineOfferDetails, error) {
	return nil, nil
}

func (happyRepo) GetVendorShipment(ctx context.Context, shipmentID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	return nil, nil
}
func (happyRepo) GetVendorShipmentByOrderID(ctx context.Context, orderID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	return nil, nil
}
func (happyRepo) AssignShipmentCourier(ctx context.Context, shipmentID, vendorOrgID int64, courierUserID *int64, assignedBy int64) error {
	return nil
}
func (happyRepo) ListCourierQueue(ctx context.Context, f commerce.CourierQueueFilter) ([]*commerce.OrderShipment, int, error) {
	return nil, 0, nil
}
func (happyRepo) CourierQueueCounts(ctx context.Context, vendorOrgID, courierUserID int64) (commerce.CourierQueueCounts, error) {
	return commerce.CourierQueueCounts{}, nil
}
func (happyRepo) ListCourierWorkload(ctx context.Context, vendorOrgID int64) ([]*commerce.CourierWorkload, error) {
	return nil, nil
}
