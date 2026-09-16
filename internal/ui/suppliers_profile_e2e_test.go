package ui_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// mockCommerceRepoForSupplierCartTest implements commerce.Repository for cart tests.
type mockCommerceRepoForSupplierCartTest struct {
	commerce.Repository
	carts     map[int64]*commerce.Cart
	cartItems map[int64][]*commerce.CartItem
	nextID    int64
}

func newMockCommerceRepo() *mockCommerceRepoForSupplierCartTest {
	return &mockCommerceRepoForSupplierCartTest{
		carts:     make(map[int64]*commerce.Cart),
		cartItems: make(map[int64][]*commerce.CartItem),
		nextID:    1,
	}
}

func (m *mockCommerceRepoForSupplierCartTest) CreateOrder(ctx context.Context, order *commerce.Order, shipments []*commerce.OrderShipment, lines []*commerce.OrderLine) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetOrderByID(ctx context.Context, id int64) (*commerce.Order, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetOrderByNumber(ctx context.Context, number string) (*commerce.Order, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListOrdersByCustomerWithTotal(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *mockCommerceRepoForSupplierCartTest) CountOrders(ctx context.Context) (int, error) {
	return 0, nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListShipmentsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.OrderShipment, int, error) {
	return nil, 0, nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetShipmentByID(ctx context.Context, id int64) (*commerce.OrderShipment, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) VerifyAndCompleteDelivery(ctx context.Context, shipmentID int64, deliveryCode, notes string, collectedAmountMinor int64) (*commerce.OrderShipment, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdateShipmentStatus(ctx context.Context, id int64, from, to commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListOrderHistory(ctx context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) RateOrder(ctx context.Context, orderID int64, customerID int64, rating float64, review string) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) AddToWishlist(ctx context.Context, userID int64, productID int64) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) CountVendorShipmentsByStatus(ctx context.Context, orgID int64, statuses []string) (int, error) {
	return 0, nil
}
func (m *mockCommerceRepoForSupplierCartTest) MonthSalesByVendor(ctx context.Context, vendorOrgID int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *mockCommerceRepoForSupplierCartTest) CountOrderLines(context.Context, []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (m *mockCommerceRepoForSupplierCartTest) MonthSpendByCustomer(ctx context.Context, customerID int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdateOrderStatus(ctx context.Context, id int64, status commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdateOrderPaymentStatus(ctx context.Context, id int64, paymentStatus commerce.PaymentStatus) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdateCustomerPendingOrder(ctx context.Context, order *commerce.Order, lines []commerce.OrderLineEditItem, changedByUserID int64) (*commerce.Order, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) SetShipmentTracking(ctx context.Context, id int64, carrier, tracking string) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) CreateQuoteRequest(ctx context.Context, q *commerce.QuoteRequest) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetQuoteRequestByID(ctx context.Context, id int64) (*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdateQuoteStatus(ctx context.Context, id int64, status commerce.QuoteStatus, price money.Amount, notes string) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListQuoteRequestsByOrg(ctx context.Context, orgID int64, isVendor bool, limit, offset int) ([]*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) CreatePurchaseRequest(ctx context.Context, pr *commerce.PurchaseRequest, lines []*commerce.PurchaseRequestLine) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetPurchaseRequestByID(ctx context.Context, id int64) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetPurchaseRequestByNumber(ctx context.Context, number string) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListPurchaseRequestsByVendor(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) CountPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64) (map[string]int, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdatePurchaseRequestStatus(ctx context.Context, id int64, status commerce.PurchaseRequestStatus, notes string, responderID *int64) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdatePurchaseRequestLineOffer(ctx context.Context, lineID int64, price money.Amount, discount float64, status string) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) AdminSearchOrdersWithTotal(ctx context.Context, query, tab string, limit, offset int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *mockCommerceRepoForSupplierCartTest) AdminSearchOrdersFiltered(ctx context.Context, filter commerce.AdminOrderFilter) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *mockCommerceRepoForSupplierCartTest) AdminOrderStats(ctx context.Context) (int, int, int, error) {
	return 0, 0, 0, nil
}
func (m *mockCommerceRepoForSupplierCartTest) AdminOrderKPIs(ctx context.Context) (commerce.AdminOrderKPIs, error) {
	return commerce.AdminOrderKPIs{}, nil
}
func (m *mockCommerceRepoForSupplierCartTest) AcceptNegotiation(ctx context.Context, orderID int64, actorID int64) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListVendorNegotiationOrdersWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string, filterOpt ...commerce.VendorFinancialFilter) (*commerce.VendorFinancialSummary, error) {
	return &commerce.VendorFinancialSummary{Period: period}, nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListWishlist(ctx context.Context, userID int64) ([]*commerce.WishlistItem, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetOrCreateCart(ctx context.Context, userID int64) (*commerce.Cart, error) {
	for _, c := range m.carts {
		if c.UserID == userID {
			return c, nil
		}
	}
	c := &commerce.Cart{
		ID:        m.nextID,
		PublicID:  fmt.Sprintf("cart_%d", m.nextID),
		UserID:    userID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.nextID++
	m.carts[c.ID] = c
	return c, nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetCartWithItems(ctx context.Context, cartID int64) (*commerce.Cart, error) {
	c, ok := m.carts[cartID]
	if !ok {
		return nil, nil
	}
	items := m.cartItems[cartID]
	res := *c
	res.Items = items
	return &res, nil
}
func (m *mockCommerceRepoForSupplierCartTest) AddToCartItem(ctx context.Context, cartID int64, item *commerce.CartItem) error {
	existing := m.cartItems[cartID]
	for _, it := range existing {
		if it.ProductVariantID == item.ProductVariantID {
			it.Quantity += item.Quantity
			it.UnitPrice = item.UnitPrice
			return nil
		}
	}
	item.CartID = cartID
	item.ID = int64(len(existing) + 1)
	m.cartItems[cartID] = append(m.cartItems[cartID], item)
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) SetCartItemQuantity(ctx context.Context, cartID int64, variantID int64, quantity int) error {
	for _, it := range m.cartItems[cartID] {
		if it.ProductVariantID == variantID {
			it.Quantity = quantity
			return nil
		}
	}
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) RemoveCartItem(ctx context.Context, cartID int64, variantID int64) error {
	var kept []*commerce.CartItem
	for _, it := range m.cartItems[cartID] {
		if it.ProductVariantID != variantID {
			kept = append(kept, it)
		}
	}
	m.cartItems[cartID] = kept
	return nil
}

func (m *mockCommerceRepoForSupplierCartTest) RemoveCartItemByID(_ context.Context, _, _ int64) error {
	return nil
}

func (m *mockCommerceRepoForSupplierCartTest) SetCartItemQuantityByID(_ context.Context, _, _ int64, _ int) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) ClearCart(ctx context.Context, cartID int64) error {
	delete(m.cartItems, cartID)
	return nil
}

type mockAvailabilityProbe struct {
	availQty int
}

func (m *mockAvailabilityProbe) Variant(ctx context.Context, variantID int64) (commerce.VariantAvailability, error) {
	return commerce.VariantAvailability{
		ID:             variantID,
		OrganizationID: 10,
		StockQty:       m.availQty,
		MinOrderQty:    1,
		Active:         true,
	}, nil
}
func (m *mockAvailabilityProbe) Vendor(ctx context.Context, orgID int64) (commerce.VendorAvailability, error) {
	return commerce.VendorAvailability{
		ID:       orgID,
		IsVendor: true,
		Approved: true,
	}, nil
}
func (m *mockAvailabilityProbe) CustomerBranch(ctx context.Context, branchID int64) (commerce.BranchAvailability, error) {
	lat := 30.0
	lon := 31.0
	return commerce.BranchAvailability{
		ID:                 branchID,
		OrganizationID:     99,
		Latitude:           &lat,
		Longitude:          &lon,
		InstitutionalWorks: []string{"commercial"},
	}, nil
}
func (m *mockAvailabilityProbe) VendorCovers(ctx context.Context, vendorOrgID, vendorBranchID int64, lat, lon float64, day time.Weekday, cityID *int64) (bool, error) {
	return true, nil
}
func (m *mockAvailabilityProbe) VendorInstitutionalConnection(ctx context.Context, vendorOrgID int64, customerBranchID int64, variantID int64) (bool, error) {
	return true, nil
}
func (m *mockAvailabilityProbe) VariantsByIDs(ctx context.Context, variantIDs []int64) (map[int64]commerce.VariantAvailability, error) {
	out := make(map[int64]commerce.VariantAvailability, len(variantIDs))
	for _, id := range variantIDs {
		out[id] = commerce.VariantAvailability{
			ID:             id,
			OrganizationID: 10,
			StockQty:       m.availQty,
			MinOrderQty:    1,
			Active:         true,
		}
	}
	return out, nil
}
func (m *mockAvailabilityProbe) VendorsByIDs(ctx context.Context, orgIDs []int64) (map[int64]commerce.VendorAvailability, error) {
	out := make(map[int64]commerce.VendorAvailability, len(orgIDs))
	for _, id := range orgIDs {
		out[id] = commerce.VendorAvailability{
			ID:       id,
			IsVendor: true,
			Approved: true,
		}
	}
	return out, nil
}
func (m *mockAvailabilityProbe) VendorInstitutionalConnections(ctx context.Context, customerBranchID int64, lines []commerce.AvailabilityLine) (map[int64]bool, error) {
	out := make(map[int64]bool, len(lines))
	for _, l := range lines {
		out[l.VariantID] = true
	}
	return out, nil
}

// TestSupplierProfileData_AvailabilityAndStock verifies view model helpers.
func TestSupplierProfileData_AvailabilityAndStock(t *testing.T) {
	v1 := &catalog.ProductVariant{
		ID:          101,
		ProductID:   1,
		StockQty:    50,
		MinOrderQty: 5,
		Price:       money.FromMajor(120),
	}
	v2 := &catalog.ProductVariant{
		ID:          102,
		ProductID:   2,
		StockQty:    0,
		MinOrderQty: 1,
		Price:       money.FromMajor(80),
	}

	data := &pages.SupplierProfileData{
		Org: &org.Organization{
			ID:        10,
			LegalName: "United Pharma Co",
			TradeName: i18n.Text{"ar": "شركة المتحدون للأدوية", "en": "United Pharma"},
		},
		Variants: []*catalog.ProductVariant{v1, v2},
		VariantMeta: map[int64]pages.SupplierVariantMeta{
			101: {
				AvailableStock: 50,
				MinOrderQty:    5,
				IsCovered:      true,
				CanAddToCart:   true,
			},
			102: {
				AvailableStock: 0,
				MinOrderQty:    1,
				IsCovered:      false,
				CoverageReason: "خارج نطاق التغطية",
				CanAddToCart:   false,
			},
		},
	}

	// Verify v1 (in stock, covered)
	if stock := data.GetAvailableStock(v1); stock != 50 {
		t.Errorf("expected available stock 50, got %d", stock)
	}
	if minQty := data.GetMinOrderQty(v1); minQty != 5 {
		t.Errorf("expected min order qty 5, got %d", minQty)
	}
	if !data.IsVariantCovered(v1) {
		t.Errorf("expected v1 to be covered")
	}
	if !data.CanAddToCart(v1) {
		t.Errorf("expected v1 to be purchasable")
	}

	// Verify v2 (0 stock, not covered)
	if stock := data.GetAvailableStock(v2); stock != 0 {
		t.Errorf("expected available stock 0, got %d", stock)
	}
	if data.IsVariantCovered(v2) {
		t.Errorf("expected v2 not to be covered")
	}
	if data.GetCoverageReason(v2) != "خارج نطاق التغطية" {
		t.Errorf("unexpected coverage reason: %s", data.GetCoverageReason(v2))
	}
	if data.CanAddToCart(v2) {
		t.Errorf("expected v2 NOT to be purchasable")
	}
}
