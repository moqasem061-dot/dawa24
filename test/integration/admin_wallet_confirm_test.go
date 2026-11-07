package integration_test

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// B2: an admin confirming a wallet shipment must charge the buyer's wallet the way a
// supplier's confirmation does. The admin handler resolves the payer and calls the
// wallet variant; the commerce service refuses a confirmation without a payer.
func TestAdminConfirmOfWalletShipmentChargesTheBuyerWallet(t *testing.T) {
	w := newWRWorld(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg}, []int64{50000}, []string{"pending"}, "pending", false)
	admin := w.buyerUser
	ctx := database.WithTenant(database.AsSystem(context.Background()), w.vendorOrg)

	if _, err := w.commSvc.TransitionShipmentStatusWithWallet(ctx, ships[0], commerce.StatusConfirmed, &admin, "admin confirm", w.buyerUser); err != nil {
		t.Fatalf("admin confirm with the buyer's wallet: %v", err)
	}
	if s := w.shipmentStatus(t, ships[0]); s != "confirmed" {
		t.Fatalf("shipment status %q, want confirmed", s)
	}
	if got := w.balance(t); got != before-50000 {
		t.Fatalf("balance %d after admin confirm, want %d (500.00)", got, before-50000)
	}
	if n := w.debitCount(t, orderID); n != 1 {
		t.Fatalf("%d wallet debits after admin confirm, want 1", n)
	}
}

// Without a payer the wallet confirmation is refused and nothing moves, so an
// admin path that forgets the payer fails closed instead of confirming unpaid.
func TestConfirmWalletShipmentWithoutPayerChangesNothing(t *testing.T) {
	w := newWRWorld(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg}, []int64{50000}, []string{"pending"}, "pending", false)
	admin := w.buyerUser
	ctx := database.AsSystem(context.Background())

	if _, err := w.commSvc.TransitionShipmentStatus(ctx, ships[0], commerce.StatusConfirmed, &admin, "no payer"); err == nil {
		t.Fatal("wallet shipment confirmed without a payer")
	}
	if s := w.shipmentStatus(t, ships[0]); s != "pending" {
		t.Fatalf("shipment status %q after a refused confirm, want pending", s)
	}
	if got := w.balance(t); got != before || w.debitCount(t, orderID) != 0 {
		t.Fatalf("a refused confirm moved money: balance %d, want %d", got, before)
	}
}
