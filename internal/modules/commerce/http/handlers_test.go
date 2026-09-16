package http_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) GetOrCreateCart(ctx context.Context, userID int64) (*commerce.Cart, error) {
	r.fail("GetOrCreateCart")
	return nil, nil
}
func (r stubRepo) GetCartWithItems(ctx context.Context, cartID int64) (*commerce.Cart, error) {
	r.fail("GetCartWithItems")
	return nil, nil
}
func (r stubRepo) AddToCartItem(ctx context.Context, cartID int64, item *commerce.CartItem) error {
	r.fail("AddToCartItem")
	return nil
}
func (r stubRepo) SetCartItemQuantity(ctx context.Context, cartID int64, variantID int64, quantity int) error {
	r.fail("SetCartItemQuantity")
	return nil
}
func (r stubRepo) RemoveCartItemByID(ctx context.Context, cartID, itemID int64) error {
	r.fail("RemoveCartItemByID")
	return nil
}

func (r stubRepo) SetCartItemQuantityByID(ctx context.Context, cartID, itemID int64, qty int) error {
	r.fail("SetCartItemQuantityByID")
	return nil
}

func (r stubRepo) RemoveCartItem(ctx context.Context, cartID int64, variantID int64) error {
	r.fail("RemoveCartItem")
	return nil
}
func (r stubRepo) ClearCart(ctx context.Context, cartID int64) error {
	r.fail("ClearCart")
	return nil
}

func (r stubRepo) CreateOrder(ctx context.Context, order *commerce.Order, shipments []*commerce.OrderShipment, lines []*commerce.OrderLine) error {
	r.fail("CreateOrder")
	return nil
}
func (r stubRepo) GetOrderByID(ctx context.Context, id int64) (*commerce.Order, error) {
	r.fail("GetOrderByID")
	return nil, nil
}
func (r stubRepo) GetOrderByNumber(ctx context.Context, number string) (*commerce.Order, error) {
	r.fail("GetOrderByNumber")
	return nil, nil
}
func (r stubRepo) UpdateOrderStatus(ctx context.Context, orderID int64, toStatus commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	r.fail("UpdateOrderStatus")
	return nil
}
func (r stubRepo) UpdateOrderPaymentStatus(ctx context.Context, orderID int64, paymentStatus commerce.PaymentStatus) error {
	r.fail("UpdateOrderPaymentStatus")
	return nil
}
func (r stubRepo) UpdateCustomerPendingOrder(ctx context.Context, order *commerce.Order, lines []commerce.OrderLineEditItem, changedByUserID int64) (*commerce.Order, error) {
	r.fail("UpdateCustomerPendingOrder")
	return nil, nil
}
func (r stubRepo) ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, error) {
	r.fail("ListOrdersByCustomer")
	return nil, nil
}
func (r stubRepo) ListOrdersByCustomerWithTotal(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, int, error) {
	r.fail("ListOrdersByCustomerWithTotal")
	return nil, 0, nil
}
func (r stubRepo) ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	r.fail("ListShipmentsByVendor")
	return nil, nil
}
func (r stubRepo) ListShipmentsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.OrderShipment, int, error) {
	r.fail("ListShipmentsByVendorWithTotal")
	return nil, 0, nil
}
func (r stubRepo) GetShipmentByID(ctx context.Context, id int64) (*commerce.OrderShipment, error) {
	r.fail("GetShipmentByID")
	return nil, nil
}
func (r stubRepo) UpdateShipmentStatus(ctx context.Context, id int64, from, to commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	r.fail("UpdateShipmentStatus")
	return nil
}
func (r stubRepo) VerifyAndCompleteDelivery(ctx context.Context, shipmentID int64, deliveryCode string, notes string, collectedAmountMinor int64) (*commerce.OrderShipment, error) {
	r.fail("VerifyAndCompleteDelivery")
	return nil, nil
}
func (r stubRepo) ListOrderHistory(ctx context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	r.fail("ListOrderHistory")
	return nil, nil
}
func (r stubRepo) RateOrder(ctx context.Context, orderID int64, customerID int64, rating float64, review string) error {
	r.fail("RateOrder")
	return nil
}
func (r stubRepo) GetOfferDetailsForOrderLine(ctx context.Context, orderID, lineID int64) (*commerce.OrderLineOfferDetails, error) {
	r.fail("GetOfferDetailsForOrderLine")
	return nil, nil
}

func (r stubRepo) AddToWishlist(ctx context.Context, userID int64, productID int64) error {
	r.fail("AddToWishlist")
	return nil
}
func (r stubRepo) RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error {
	r.fail("RemoveFromWishlist")
	return nil
}
func (r stubRepo) ListWishlist(ctx context.Context, userID int64) ([]*commerce.WishlistItem, error) {
	r.fail("ListWishlist")
	return nil, nil
}

func (r stubRepo) CreateQuoteRequest(ctx context.Context, qr *commerce.QuoteRequest) error {
	r.fail("CreateQuoteRequest")
	return nil
}
func (r stubRepo) GetQuoteRequestByID(ctx context.Context, id int64) (*commerce.QuoteRequest, error) {
	r.fail("GetQuoteRequestByID")
	return nil, nil
}
func (r stubRepo) UpdateQuoteStatus(ctx context.Context, id int64, status commerce.QuoteStatus, quotePrice money.Amount, supplierNotes string) error {
	r.fail("UpdateQuoteStatus")
	return nil
}
func (r stubRepo) ListQuoteRequestsByOrg(ctx context.Context, orgID int64, asSupplier bool, limit, offset int) ([]*commerce.QuoteRequest, error) {
	r.fail("ListQuoteRequestsByOrg")
	return nil, nil
}

func (r stubRepo) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	r.fail("AdminSearchOrders")
	return nil, nil
}
func (r stubRepo) AdminSearchOrdersWithTotal(ctx context.Context, query, tab string, limit, offset int) ([]*commerce.Order, int, error) {
	r.fail("AdminSearchOrdersWithTotal")
	return nil, 0, nil
}
func (r stubRepo) AdminSearchOrdersFiltered(ctx context.Context, filter commerce.AdminOrderFilter) ([]*commerce.Order, int, error) {
	r.fail("AdminSearchOrdersFiltered")
	return nil, 0, nil
}
func (r stubRepo) AdminOrderStats(ctx context.Context) (int, int, int, error) {
	r.fail("AdminOrderStats")
	return 0, 0, 0, nil
}
func (r stubRepo) AdminOrderKPIs(ctx context.Context) (commerce.AdminOrderKPIs, error) {
	r.fail("AdminOrderKPIs")
	return commerce.AdminOrderKPIs{}, nil
}
func (r stubRepo) CreatePurchaseRequest(ctx context.Context, pr *commerce.PurchaseRequest, lines []*commerce.PurchaseRequestLine) error {
	r.fail("CreatePurchaseRequest")
	return nil
}
func (r stubRepo) GetPurchaseRequestByID(ctx context.Context, id int64) (*commerce.PurchaseRequest, error) {
	r.fail("GetPurchaseRequestByID")
	return nil, nil
}
func (r stubRepo) GetPurchaseRequestByNumber(ctx context.Context, number string) (*commerce.PurchaseRequest, error) {
	r.fail("GetPurchaseRequestByNumber")
	return nil, nil
}
func (r stubRepo) ListPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	r.fail("ListPurchaseRequestsByCustomer")
	return nil, nil
}
func (r stubRepo) ListPurchaseRequestsByVendor(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	r.fail("ListPurchaseRequestsByVendor")
	return nil, nil
}
func (r stubRepo) ListPurchaseRequestsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, int, error) {
	r.fail("ListPurchaseRequestsByVendorWithTotal")
	return nil, 0, nil
}
func (r stubRepo) CountPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64) (map[string]int, error) {
	r.fail("CountPurchaseRequestsByCustomer")
	return nil, nil
}
func (r stubRepo) UpdatePurchaseRequestStatus(ctx context.Context, id int64, status commerce.PurchaseRequestStatus, vendorNotes string, responderID *int64) error {
	r.fail("UpdatePurchaseRequestStatus")
	return nil
}
func (r stubRepo) UpdatePurchaseRequestLineOffer(ctx context.Context, lineID int64, price money.Amount, discount float64, status string) error {
	r.fail("UpdatePurchaseRequestLineOffer")
	return nil
}
func (r stubRepo) AcceptNegotiation(ctx context.Context, orderID int64, actorID int64) error {
	r.fail("AcceptNegotiation")
	return nil
}
func (r stubRepo) RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error {
	r.fail("RejectNegotiation")
	return nil
}
func (r stubRepo) ListVendorNegotiationOrdersWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.Order, int, error) {
	r.fail("ListVendorNegotiationOrdersWithTotal")
	return nil, 0, nil
}
func (r stubRepo) GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string, filterOpt ...commerce.VendorFinancialFilter) (*commerce.VendorFinancialSummary, error) {
	r.fail("GetVendorFinancialSummary")
	return nil, nil
}

func (r stubRepo) GetVendorShipment(ctx context.Context, shipmentID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	r.fail("GetVendorShipment")
	return nil, nil
}
func (r stubRepo) GetVendorShipmentByOrderID(ctx context.Context, orderID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	r.fail("GetVendorShipmentByOrderID")
	return nil, nil
}
func (r stubRepo) AssignShipmentCourier(ctx context.Context, shipmentID, vendorOrgID int64, courierUserID *int64, assignedBy int64) error {
	r.fail("AssignShipmentCourier")
	return nil
}
func (r stubRepo) ListCourierQueue(ctx context.Context, f commerce.CourierQueueFilter) ([]*commerce.OrderShipment, int, error) {
	r.fail("ListCourierQueue")
	return nil, 0, nil
}
func (r stubRepo) CourierQueueCounts(ctx context.Context, vendorOrgID, courierUserID int64) (commerce.CourierQueueCounts, error) {
	r.fail("CourierQueueCounts")
	return commerce.CourierQueueCounts{}, nil
}
func (r stubRepo) ListCourierWorkload(ctx context.Context, vendorOrgID int64) ([]*commerce.CourierWorkload, error) {
	r.fail("ListCourierWorkload")
	return nil, nil
}

