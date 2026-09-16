package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// What a delivery representative is told to collect when the pharmacy paid
// from its organisation wallet.
//
// The goods are already settled, so the only cash owed is the delivery fee —
// and where a supplier delivers free, nothing at all. Getting this wrong in
// either direction is a real loss: a courier who collects an invoice that was
// already paid, or one who hands over stock and collects nothing.

type walletMockCommerceRepo struct {
	commerce.Repository
	shipment *commerce.OrderShipment
}

func (m *walletMockCommerceRepo) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == id {
		c := *m.shipment
		return &c, nil
	}
	return nil, apperr.NotFound("shipment")
}

func (m *walletMockCommerceRepo) GetVendorShipment(_ context.Context, shipmentID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == shipmentID && m.shipment.OrganizationID == vendorOrgID {
		c := *m.shipment
		return &c, nil
	}
	return nil, apperr.NotFound("shipment")
}

func (m *walletMockCommerceRepo) VerifyAndCompleteDelivery(
	_ context.Context,
	shipmentID int64,
	deliveryCode string,
	notes string,
	collectedAmountMinor int64,
) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == shipmentID {
		m.shipment.Status = commerce.StatusDelivered
		m.shipment.CollectedAmountMinor = collectedAmountMinor
		return m.shipment, nil
	}
	return nil, apperr.NotFound("shipment")
}

func (m *walletMockCommerceRepo) GetOrderByID(_ context.Context, id int64) (*commerce.Order, error) {
	return &commerce.Order{ID: id, OrderNumber: "ORD-WALLET-100", CustomerID: 10}, nil
}

// walletShipment is a parcel already on the assigned courier's round, so the
// parcel screen renders rather than refusing.
func walletShipment(id int64, shippingFee, total money.Amount) *commerce.OrderShipment {
	courier := assignedCourierID
	return &commerce.OrderShipment{
		ID:              id,
		OrganizationID:  deliveryOrgID,
		OrderID:         202,
		ShipmentNumber:  "SH-WALLET-001",
		TrackingNumber:  "TRK-WALLET-001",
		Status:          commerce.StatusOutForDelivery,
		DeliveryCode:    "123456",
		PaymentMethod:   "wallet",
		PaymentStatus:   commerce.PaymentPaid,
		Subtotal:        money.FromMinor(50000),
		ShippingFee:     shippingFee,
		TotalAmount:     total,
		CourierUserID:   &courier,
		VendorName:      i18n.New("شركة المتحدة للتوزيع", "United Distribution"),
		CustomerOrgName: i18n.New("صيدلية الشفاء", "Al-Shifa Pharmacy"),
	}
}

func TestDeliveryScreenWalletPaymentCollectsShippingOnly(t *testing.T) {
	// 500.00 of goods paid from the wallet, plus a 35.00 delivery fee.
	repo := &walletMockCommerceRepo{
		shipment: walletShipment(101, money.FromMinor(3500), money.FromMinor(53500)),
	}
	h := deliveryHandler(repo)

	rr := httptest.NewRecorder()
	h.VendorDeliveryShipmentPage(rr, deliveryRequest(t, http.MethodGet, "/vendor/delivery/101", nil,
		courierActor(assignedCourierID), map[string]string{"id": "101"}))
	if rr.Code != http.StatusOK {
		t.Fatalf("parcel screen returned %d, want 200", rr.Code)
	}
	body := rr.Body.String()

	if !strings.Contains(body, "مسدد بالمحفظة المؤسسية مسبقاً") {
		t.Error("the screen does not say the goods were paid from the wallet")
	}
	if !strings.Contains(body, "رسوم التوصيل فقط") {
		t.Error("the screen does not say only the delivery fee is collected")
	}
	if !strings.Contains(body, "35.00") || !strings.Contains(body, "collected_amount") {
		t.Error("the handover collection does not provide the 35.00 delivery fee input")
	}
	// The full invoice must not appear as an amount to collect.
	if strings.Contains(body, "value=\"535.00\"") {
		t.Error("the courier is asked to collect the whole prepaid invoice")
	}
}

func TestDeliveryScreenWalletPaymentWithFreeShippingCollectsNothing(t *testing.T) {
	repo := &walletMockCommerceRepo{
		shipment: walletShipment(102, money.Zero, money.FromMinor(50000)),
	}
	h := deliveryHandler(repo)

	rr := httptest.NewRecorder()
	h.VendorDeliveryShipmentPage(rr, deliveryRequest(t, http.MethodGet, "/vendor/delivery/102", nil,
		courierActor(assignedCourierID), map[string]string{"id": "102"}))
	if rr.Code != http.StatusOK {
		t.Fatalf("parcel screen returned %d, want 200", rr.Code)
	}
	body := rr.Body.String()

	if !strings.Contains(body, "مدفوع بالكامل مسبقاً") {
		t.Error("the screen does not say the parcel is fully prepaid")
	}
	if !strings.Contains(body, "لا تلزم أي مبالغ نقدية") {
		t.Error("the screen does not say no cash is due")
	}
	// A confirmation checkbox that always appears is one people always tick,
	// so there must not be one when there is nothing to collect.
	if strings.Contains(body, "أؤكد تحصيل") {
		t.Error("a cash-collection confirmation is shown for a parcel with nothing to collect")
	}
}

// TestWalletCollectionRuleIsOneRule holds the domain rule the screen renders
// and the completion transaction records to the same three answers.
func TestWalletCollectionRuleIsOneRule(t *testing.T) {
	cases := []struct {
		name     string
		shipment *commerce.OrderShipment
		wantKind commerce.CollectionKind
		wantMino int64
	}{
		{
			name:     "wallet with a delivery fee collects the fee",
			shipment: walletShipment(1, money.FromMinor(3500), money.FromMinor(53500)),
			wantKind: commerce.CollectShippingOnly,
			wantMino: 3500,
		},
		{
			name:     "wallet with free shipping collects nothing",
			shipment: walletShipment(2, money.Zero, money.FromMinor(50000)),
			wantKind: commerce.CollectNothing,
			wantMino: 0,
		},
		{
			name: "cash on delivery collects the whole invoice",
			shipment: &commerce.OrderShipment{
				PaymentMethod: "cod",
				ShippingFee:   money.FromMinor(3500),
				TotalAmount:   money.FromMinor(53500),
			},
			wantKind: commerce.CollectFull,
			wantMino: 53500,
		},
		{
			name: "paid by card collects nothing",
			shipment: &commerce.OrderShipment{
				PaymentMethod: "card",
				PaymentStatus: commerce.PaymentPaid,
				TotalAmount:   money.FromMinor(53500),
			},
			wantKind: commerce.CollectNothing,
			wantMino: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.shipment.CourierCollection()
			if got.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", got.Kind, tc.wantKind)
			}
			if got.Amount.Minor() != tc.wantMino {
				t.Errorf("amount = %d minor, want %d", got.Amount.Minor(), tc.wantMino)
			}
		})
	}
}
