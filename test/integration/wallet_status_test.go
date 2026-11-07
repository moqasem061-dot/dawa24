package integration_test

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// B3: supplier A's charge is refunded in full while supplier B's shipment is still
// open and unpaid. The order is partially refunded, not refunded.
func TestRefundOfOneSupplierWithAnotherOpenIsPartiallyRefunded(t *testing.T) {
	w := newWRWorld(t)
	vendorB := w.secondVendor(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg, vendorB}, []int64{50000, 30000}, []string{"pending", "pending"}, "pending", false)
	if err := w.confirmAs(w.vendorOrg, ships[0]); err != nil {
		t.Fatalf("confirm supplier A: %v", err)
	}
	if err := w.cancelShipmentAs(w.vendorOrg, ships[0]); err != nil {
		t.Fatalf("cancel supplier A: %v", err)
	}
	if got := w.balance(t); got != before {
		t.Fatalf("balance %d after the refund, want %d", got, before)
	}
	if s := w.paymentStatus(t, orderID); s != "partially_refunded" {
		t.Fatalf("payment status %q while supplier B is still open, want partially_refunded", s)
	}
}

// B5 (re-scoped): a cancelled wallet order cannot be marked refunded. The state
// machine refuses cancelled as a terminal state, so no money moves and the order
// keeps its status. The message is the state-machine one, not the wallet one.
func TestCancelledWalletOrderCannotBeMarkedRefundedAndMovesNoMoney(t *testing.T) {
	w := newWRWorld(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg}, []int64{50000}, []string{"pending"}, "pending", false)
	if err := w.confirmAs(w.vendorOrg, ships[0]); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	ctx := database.AsSystem(context.Background())
	if err := w.commSvc.CancelOrder(ctx, orderID, &w.buyerUser, "buyer cancel"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got := w.balance(t); got != before {
		t.Fatalf("balance %d after the cancel refund, want %d", got, before)
	}
	err := w.commSvc.TransitionOrderStatus(ctx, orderID, commerce.StatusRefunded, &w.buyerUser, "admin marks refunded")
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "order.invalid_transition" {
		t.Fatalf("error %v, want order.invalid_transition", err)
	}
	if got := w.balance(t); got != before {
		t.Fatalf("balance %d after the refused status move, want %d", got, before)
	}
}
