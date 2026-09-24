package commerce_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

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
