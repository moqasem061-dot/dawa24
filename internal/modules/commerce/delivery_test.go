package commerce_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type deliveryMockRepo struct {
	shipments map[string]*commerce.OrderShipment
}

func (m *deliveryMockRepo) GetOrCreateCart(_ context.Context, _ int64) (*commerce.Cart, error) {
	return nil, nil
}
func (m *deliveryMockRepo) GetCartWithItems(_ context.Context, _ int64) (*commerce.Cart, error) {
	return nil, nil
}
func (m *deliveryMockRepo) AddToCartItem(_ context.Context, _ int64, _ *commerce.CartItem) error {
	return nil
}
func (m *deliveryMockRepo) SetCartItemQuantity(_ context.Context, _, _ int64, _ int) error {
	return nil
}
func (m *deliveryMockRepo) RemoveCartItem(_ context.Context, _, _ int64) error { return nil }

func (m *deliveryMockRepo) RemoveCartItemByID(_ context.Context, _, _ int64) error { return nil }

func (m *deliveryMockRepo) SetCartItemQuantityByID(_ context.Context, _, _ int64, _ int) error {
	return nil
}
func (m *deliveryMockRepo) ClearCart(_ context.Context, _ int64) error { return nil }
func (m *deliveryMockRepo) CreateOrder(_ context.Context, _ *commerce.Order, _ []*commerce.OrderShipment, _ []*commerce.OrderLine) error {
	return nil
}
func (m *deliveryMockRepo) GetOrderByID(_ context.Context, _ int64) (*commerce.Order, error) {
	return nil, nil
}
func (m *deliveryMockRepo) GetOrderByNumber(_ context.Context, _ string) (*commerce.Order, error) {
	return nil, nil
}
func (m *deliveryMockRepo) UpdateOrderStatus(_ context.Context, _ int64, _ commerce.OrderStatus, _ commerce.OrderStatusHistory) error {
	return nil
}
func (m *deliveryMockRepo) UpdateOrderPaymentStatus(_ context.Context, _ int64, _ commerce.PaymentStatus) error {
	return nil
}
func (m *deliveryMockRepo) UpdateCustomerPendingOrder(_ context.Context, _ *commerce.Order, _ []commerce.OrderLineEditItem, _ int64) (*commerce.Order, error) {
	return nil, nil
}
func (m *deliveryMockRepo) ListOrdersByCustomer(_ context.Context, _ int64, _, _ int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *deliveryMockRepo) ListOrdersByCustomerWithTotal(_ context.Context, _ int64, _, _ int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *deliveryMockRepo) CountOrders(_ context.Context) (int, error) { return 0, nil }
func (m *deliveryMockRepo) CountVendorShipmentsByStatus(_ context.Context, _ int64, _ []string) (int, error) {
	return 0, nil
}
func (m *deliveryMockRepo) MonthSalesByVendor(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *deliveryMockRepo) CountOrderLines(context.Context, []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (m *deliveryMockRepo) MonthSpendByCustomer(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *deliveryMockRepo) ListShipmentsByVendor(_ context.Context, _ int64, _, _ int) ([]*commerce.OrderShipment, error) {
	return nil, nil
}
func (m *deliveryMockRepo) ListShipmentsByVendorWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*commerce.OrderShipment, int, error) {
	return nil, 0, nil
}
func (m *deliveryMockRepo) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	for _, s := range m.shipments {
		if s.ID == id {
			c := *s
			return &c, nil
		}
	}
	return nil, nil
}
func (m *deliveryMockRepo) UpdateShipmentStatus(_ context.Context, id int64, _, to commerce.OrderStatus, _ commerce.OrderStatusHistory) error {
	for _, s := range m.shipments {
		if s.ID == id {
			s.Status = to
			return nil
		}
	}
	return nil
}
func (m *deliveryMockRepo) SetShipmentTracking(_ context.Context, id int64, carrier, tracking string) error {
	for _, s := range m.shipments {
		if s.ID == id {
			s.CarrierName = carrier
			s.TrackingNumber = tracking
			return nil
		}
	}
	return nil
}
func (m *deliveryMockRepo) ListOrderHistory(_ context.Context, _ int64) ([]*commerce.OrderStatusHistory, error) {
	return nil, nil
}
func (m *deliveryMockRepo) RateOrder(_ context.Context, _, _ int64, _ float64, _ string) error {
	return nil
}
func (m *deliveryMockRepo) GetOfferDetailsForOrderLine(_ context.Context, _, _ int64) (*commerce.OrderLineOfferDetails, error) {
	return nil, nil
}
func (m *deliveryMockRepo) AddToWishlist(_ context.Context, _, _ int64) error      { return nil }
func (m *deliveryMockRepo) RemoveFromWishlist(_ context.Context, _, _ int64) error { return nil }
func (m *deliveryMockRepo) ListWishlist(_ context.Context, _ int64) ([]*commerce.WishlistItem, error) {
	return nil, nil
}
func (m *deliveryMockRepo) CreateQuoteRequest(_ context.Context, _ *commerce.QuoteRequest) error {
	return nil
}
func (m *deliveryMockRepo) GetQuoteRequestByID(_ context.Context, _ int64) (*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) UpdateQuoteStatus(_ context.Context, _ int64, _ commerce.QuoteStatus, _ money.Amount, _ string) error {
	return nil
}
func (m *deliveryMockRepo) ListQuoteRequestsByOrg(_ context.Context, _ int64, _ bool, _, _ int) ([]*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) CreatePurchaseRequest(_ context.Context, _ *commerce.PurchaseRequest, _ []*commerce.PurchaseRequestLine) error {
	return nil
}
func (m *deliveryMockRepo) GetPurchaseRequestByID(_ context.Context, _ int64) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) GetPurchaseRequestByNumber(_ context.Context, _ string) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) ListPurchaseRequestsByCustomer(_ context.Context, _ int64, _ *int64, _ string, _, _ int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) ListPurchaseRequestsByVendor(_ context.Context, _ int64, _ string, _, _ int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) ListPurchaseRequestsByVendorWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*commerce.PurchaseRequest, int, error) {
	return nil, 0, nil
}
func (m *deliveryMockRepo) CountPurchaseRequestsByCustomer(_ context.Context, _ int64, _ *int64) (map[string]int, error) {
	return nil, nil
}
func (m *deliveryMockRepo) UpdatePurchaseRequestStatus(_ context.Context, _ int64, _ commerce.PurchaseRequestStatus, _ string, _ *int64) error {
	return nil
}
func (m *deliveryMockRepo) UpdatePurchaseRequestLineOffer(_ context.Context, _ int64, _ money.Amount, _ float64, _ string) error {
	return nil
}
func (m *deliveryMockRepo) AdminSearchOrders(_ context.Context, _ string, _, _ int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *deliveryMockRepo) AdminSearchOrdersWithTotal(_ context.Context, _, _ string, _, _ int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *deliveryMockRepo) AdminSearchOrdersFiltered(_ context.Context, _ commerce.AdminOrderFilter) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *deliveryMockRepo) AdminOrderStats(_ context.Context) (int, int, int, error) {
	return 0, 0, 0, nil
}
func (m *deliveryMockRepo) AdminOrderKPIs(_ context.Context) (commerce.AdminOrderKPIs, error) {
	return commerce.AdminOrderKPIs{}, nil
}
func (m *deliveryMockRepo) AcceptNegotiation(_ context.Context, _, _ int64) error { return nil }
func (m *deliveryMockRepo) RejectNegotiation(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}
func (m *deliveryMockRepo) ListVendorNegotiationOrdersWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}

func (m *deliveryMockRepo) VerifyAndCompleteDelivery(
	_ context.Context,
	shipmentID int64,
	deliveryCode string,
	notes string,
	collectedAmountMinor int64,
) (*commerce.OrderShipment, error) {
	for _, s := range m.shipments {
		if s.ID == shipmentID {
			now := time.Now()
			if s.DeliveryLockedUntil != nil && s.DeliveryLockedUntil.After(now) {
				return nil, commerce.ErrDeliveryLocked
			}
			if s.DeliveryCode != "" && s.DeliveryCode != deliveryCode {
				s.DeliveryAttempts++
				if s.DeliveryAttempts >= 5 {
					lockUntil := now.Add(15 * time.Minute)
					s.DeliveryLockedUntil = &lockUntil
					return nil, commerce.ErrDeliveryLocked
				}
				return nil, commerce.ErrInvalidDeliveryCode
			}
			s.Status = commerce.StatusDelivered
			s.DeliveredAt = &now
			s.DeliveredByCourierAt = &now
			s.DeliveryNotes = notes
			// Zero means "work it out", exactly as the SQL implementation
			// does: the amount recorded is the amount the courier's screen
			// told them to collect, not a number a caller supplied.
			if collectedAmountMinor < 0 {
				collectedAmountMinor = s.CourierCollection().Amount.Minor()
			}
			s.CollectedAmountMinor = collectedAmountMinor
			s.DeliveryAttempts = 0
			s.DeliveryLockedUntil = nil
			c := *s
			return &c, nil
		}
	}
	return nil, nil
}

func (m *deliveryMockRepo) GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string, filterOpt ...commerce.VendorFinancialFilter) (*commerce.VendorFinancialSummary, error) {
	return &commerce.VendorFinancialSummary{Period: period}, nil
}

func TestCourierDeliveryPINAndTrackingHelpers(t *testing.T) {
	pin1 := commerce.GenerateDeliveryCode()
	pin2 := commerce.GenerateDeliveryCode()
	if len(pin1) != 6 || len(pin2) != 6 {
		t.Fatalf("expected 6-digit delivery PIN, got %s, %s", pin1, pin2)
	}

	trk := commerce.GenerateTrackingNumber("ORD-100", 1)
	if len(trk) < 8 || trk[:3] != "TRK" {
		t.Fatalf("expected formatted tracking number, got %s", trk)
	}
}

// TestCourierDeliveryLifecycle_Success walks the handover a delivery
// representative performs: they open a parcel on their own round, enter the
// pharmacy's code, and the parcel closes with the cash recorded.
//
// The lookup is by shipment id scoped to the supplier, not by waybill number.
// The waybill path existed and was removed with the unlisted portal: knowing a
// tracking number was enough to read a pharmacy's address and close its order,
// and no test could express "the wrong person did this" because the code had
// no notion of the right one.
func TestCourierDeliveryLifecycle_Success(t *testing.T) {
	const (
		vendorOrgID int64 = 10
		courierID   int64 = 55
	)
	courier := courierID
	repo := &deliveryMockRepo{
		shipments: map[string]*commerce.OrderShipment{
			"TRK-123456": {
				ID:             1,
				OrganizationID: vendorOrgID,
				ShipmentNumber: "SH-1",
				TrackingNumber: "TRK-123456",
				Status:         commerce.StatusShipped,
				DeliveryCode:   "482915",
				PaymentMethod:  "cod",
				TotalAmount:    money.FromMinor(54000),
				CourierUserID:  &courier,
			},
		},
	}

	svc := commerce.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := database.WithTenant(context.Background(), vendorOrgID)

	// 1. The representative opens the parcel on their own round.
	sh, err := svc.GetCourierShipment(ctx, 1, vendorOrgID, courierID)
	if err != nil || sh == nil {
		t.Fatalf("GetCourierShipment failed: %v", err)
	}
	if sh.ShipmentNumber != "SH-1" {
		t.Errorf("got shipment number %s, want SH-1", sh.ShipmentNumber)
	}

	// 2. Somebody else on the same supplier cannot.
	if _, err := svc.GetCourierShipment(ctx, 1, vendorOrgID, courierID+1); err == nil {
		t.Error("a courier opened a parcel assigned to somebody else")
	}

	// 3. The pharmacy's code closes it.
	completed, err := svc.CompleteCourierDelivery(ctx, 1, vendorOrgID, courierID, "482915", "تم التحصيل كاش")
	if err != nil || completed == nil {
		t.Fatalf("CompleteCourierDelivery failed: %v", err)
	}
	if completed.Status != commerce.StatusDelivered {
		t.Errorf("status = %v, want delivered", completed.Status)
	}
	if completed.DeliveredByCourierAt == nil {
		t.Error("expected DeliveredByCourierAt to be set")
	}
	if completed.DeliveryNotes != "تم التحصيل كاش" {
		t.Errorf("delivery notes = %q, want the note that was passed", completed.DeliveryNotes)
	}
	// Cash on delivery: the whole invoice, taken from the domain rule rather
	// than from a number the caller supplied.
	if completed.CollectedAmountMinor != 54000 {
		t.Errorf("collected %d minor, want 54000", completed.CollectedAmountMinor)
	}
}

// TestCourierDeliveryRefusesTheWrongCode.
func TestCourierDeliveryRefusesTheWrongCode(t *testing.T) {
	const vendorOrgID, courierID int64 = 10, 55
	courier := courierID
	repo := &deliveryMockRepo{
		shipments: map[string]*commerce.OrderShipment{
			"TRK-123456": {
				ID: 1, OrganizationID: vendorOrgID, ShipmentNumber: "SH-1",
				TrackingNumber: "TRK-123456", Status: commerce.StatusShipped,
				DeliveryCode: "482915", TotalAmount: money.FromMinor(54000),
				CourierUserID: &courier,
			},
		},
	}
	svc := commerce.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := database.WithTenant(context.Background(), vendorOrgID)

	if _, err := svc.CompleteCourierDelivery(ctx, 1, vendorOrgID, courierID, "000000", ""); err == nil {
		t.Fatal("a wrong code closed the parcel")
	}
	if _, err := svc.CompleteCourierDelivery(ctx, 1, vendorOrgID, courierID, "", ""); err == nil {
		t.Fatal("an empty code closed the parcel")
	}
	if repo.shipments["TRK-123456"].Status == commerce.StatusDelivered {
		t.Fatal("the parcel was closed despite the refusals")
	}
}

// The dispatch board.

func (m *deliveryMockRepo) GetVendorShipment(_ context.Context, shipmentID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	for _, s := range m.shipments {
		if s.ID == shipmentID && s.OrganizationID == vendorOrgID {
			c := *s
			return &c, nil
		}
	}
	return nil, apperr.NotFound("shipment")
}

func (m *deliveryMockRepo) GetVendorShipmentByOrderID(_ context.Context, orderID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	for _, s := range m.shipments {
		if s.OrderID == orderID && s.OrganizationID == vendorOrgID {
			c := *s
			return &c, nil
		}
	}
	return nil, apperr.NotFound("shipment")
}

func (m *deliveryMockRepo) AssignShipmentCourier(_ context.Context, _, _ int64, _ *int64, _ int64) error {
	return nil
}

func (m *deliveryMockRepo) ListCourierQueue(_ context.Context, _ commerce.CourierQueueFilter) ([]*commerce.OrderShipment, int, error) {
	return nil, 0, nil
}

func (m *deliveryMockRepo) CourierQueueCounts(_ context.Context, _, _ int64) (commerce.CourierQueueCounts, error) {
	return commerce.CourierQueueCounts{}, nil
}

func (m *deliveryMockRepo) ListCourierWorkload(_ context.Context, _ int64) ([]*commerce.CourierWorkload, error) {
	return nil, nil
}

// TestCourierDelivery_CustomCashCollection verifies that when a courier manually inputs
// an amount (partial, full, or zero), it is respected accurately without forced full confirmation.
func TestCourierDelivery_CustomCashCollection(t *testing.T) {
	const vendorOrgID, courierID int64 = 10, 55
	courier := courierID
	repo := &deliveryMockRepo{
		shipments: map[string]*commerce.OrderShipment{
			"TRK-PARTIAL": {
				ID: 101, OrganizationID: vendorOrgID, ShipmentNumber: "SH-PARTIAL",
				TrackingNumber: "TRK-PARTIAL", Status: commerce.StatusShipped,
				DeliveryCode: "123456", PaymentMethod: "cod", TotalAmount: money.FromMinor(50000),
				CourierUserID: &courier,
			},
			"TRK-ZERO": {
				ID: 102, OrganizationID: vendorOrgID, ShipmentNumber: "SH-ZERO",
				TrackingNumber: "TRK-ZERO", Status: commerce.StatusShipped,
				DeliveryCode: "654321", PaymentMethod: "cod", TotalAmount: money.FromMinor(30000),
				CourierUserID: &courier,
			},
		},
	}
	svc := commerce.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := database.WithTenant(context.Background(), vendorOrgID)

	// 1. Partial collection: customer pays 200.00 EGP (20000 minor) out of 500.00 EGP (50000 minor)
	completedPartial, err := svc.CompleteCourierDelivery(ctx, 101, vendorOrgID, courierID, "123456", "استلمت 200 والباقي لاحقاً", money.FromMinor(20000))
	if err != nil || completedPartial == nil {
		t.Fatalf("CompleteCourierDelivery partial failed: %v", err)
	}
	if completedPartial.CollectedAmountMinor != 20000 {
		t.Errorf("got collected amount %d, want 20000", completedPartial.CollectedAmountMinor)
	}
	if completedPartial.Status != commerce.StatusDelivered {
		t.Errorf("status = %v, want delivered", completedPartial.Status)
	}

	// 2. Zero collection: customer will pay vendor via bank/later
	completedZero, err := svc.CompleteCourierDelivery(ctx, 102, vendorOrgID, courierID, "654321", "لم يتم الدفع كاش", money.Zero)
	if err != nil || completedZero == nil {
		t.Fatalf("CompleteCourierDelivery zero failed: %v", err)
	}
	if completedZero.CollectedAmountMinor != 0 {
		t.Errorf("got collected amount %d, want 0", completedZero.CollectedAmountMinor)
	}
	if completedZero.Status != commerce.StatusDelivered {
		t.Errorf("status = %v, want delivered", completedZero.Status)
	}
}

