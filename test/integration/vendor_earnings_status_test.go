package integration_test

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// A6: a supplier's earnings show the payment state of its own shipment. On a wallet
// order, supplier A's charged shipment reads paid while supplier B's open shipment
// is not in the list yet and the order itself is partially paid.
func TestSupplierEarningsShowItsOwnShipmentPaymentState(t *testing.T) {
	w := newWRWorld(t)
	vendorB := w.secondVendor(t)
	_, ships := w.shipmentOrder(t, []int64{w.vendorOrg, vendorB}, []int64{50000, 30000}, []string{"pending", "pending"}, "pending", false)
	if err := w.confirmAs(w.vendorOrg, ships[0]); err != nil {
		t.Fatalf("confirm supplier A: %v", err)
	}

	ctx := database.WithTenant(database.AsSystem(context.Background()), w.vendorOrg)
	summary, err := w.commSvc.GetVendorFinancialSummary(ctx, w.vendorOrg, "all")
	if err != nil {
		t.Fatalf("supplier A earnings: %v", err)
	}
	if len(summary.Shipments) != 1 {
		t.Fatalf("%d shipments in supplier A's earnings, want 1", len(summary.Shipments))
	}
	if got := summary.Shipments[0].PaymentStatus; got != "paid" {
		t.Fatalf("supplier A's shipment shows %q, want paid (its own charge), not the order's partially_paid", got)
	}
}
