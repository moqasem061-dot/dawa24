package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// A10: a negotiation order can hold shipments from more than one supplier. Each
// supplier accepts only its own part, and the order is confirmed once all of them
// have accepted.
func TestNegotiationOrderIsConfirmedOnlyAfterEverySupplierAccepts(t *testing.T) {
	w := newWRWorld(t)
	vendorB := w.secondVendor(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg, vendorB}, []int64{50000, 30000}, []string{"pending", "pending"}, "pending", true)

	if err := w.commSvc.AcceptNegotiation(context.Background(), orderID, w.vendorOrg, w.buyerUser, w.buyerUser); err != nil {
		t.Fatalf("supplier A accept: %v", err)
	}
	if s := w.orderNegotiationStatus(t, orderID); s != "pending" {
		t.Fatalf("after supplier A only: negotiation status %q, want pending", s)
	}
	if s := w.orderStatus(t, orderID); s != "pending" {
		t.Fatalf("after supplier A only: order status %q, want pending", s)
	}
	if s := w.shipmentStatus(t, ships[1]); s != "pending" {
		t.Fatalf("supplier A's accept changed supplier B's shipment to %q", s)
	}

	if err := w.commSvc.AcceptNegotiation(context.Background(), orderID, vendorB, w.buyerUser, w.buyerUser); err != nil {
		t.Fatalf("supplier B accept after supplier A: %v", err)
	}
	if s := w.orderNegotiationStatus(t, orderID); s != "accepted" {
		t.Fatalf("after both suppliers: negotiation status %q, want accepted", s)
	}
	if s := w.orderStatus(t, orderID); s != "confirmed" {
		t.Fatalf("after both suppliers: order status %q, want confirmed", s)
	}
	if got := w.balance(t); got != before-80000 {
		t.Fatalf("balance %d after both acceptances, want %d (800.00 in total)", got, before-80000)
	}
}

// Once a negotiation is accepted it can no longer be rejected: a rejection would
// cancel the order and refund its charges.
func TestRejectNegotiationAfterAcceptanceIsRefused(t *testing.T) {
	w := newWRWorld(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg}, []int64{50000}, []string{"pending"}, "pending", true)
	if err := w.commSvc.AcceptNegotiation(context.Background(), orderID, w.vendorOrg, w.buyerUser, w.buyerUser); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := w.commSvc.RejectNegotiation(context.Background(), orderID, "too late", w.buyerUser); err == nil {
		t.Fatal("accepted negotiation was rejected")
	} else {
		requireApperrCode(t, err, "order.negotiation_not_pending")
	}
	if s := w.orderStatus(t, orderID); s != "confirmed" {
		t.Fatalf("order status %q after a refused rejection, want confirmed", s)
	}
	if s := w.shipmentStatus(t, ships[0]); s != "confirmed" {
		t.Fatalf("shipment status %q after a refused rejection, want confirmed", s)
	}
	if got := w.balance(t); got != before-50000 {
		t.Fatalf("balance %d after a refused rejection, want %d (charge kept)", got, before-50000)
	}
}

// A rejected negotiation cannot be rejected again or accepted afterwards.
func TestRejectedNegotiationCannotBeRejectedTwice(t *testing.T) {
	w := newWRWorld(t)
	orderID, _ := w.shipmentOrder(t, []int64{w.vendorOrg}, []int64{50000}, []string{"pending"}, "pending", true)
	if err := w.commSvc.RejectNegotiation(context.Background(), orderID, "first", w.buyerUser); err != nil {
		t.Fatalf("first reject: %v", err)
	}
	if err := w.commSvc.RejectNegotiation(context.Background(), orderID, "second", w.buyerUser); err == nil {
		t.Fatal("rejected negotiation was rejected a second time")
	} else {
		requireApperrCode(t, err, "order.negotiation_not_pending")
	}
}

func (w *wrWorld) orderNegotiationStatus(t *testing.T, orderID int64) string {
	t.Helper()
	var s string
	err := w.db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT negotiation_status FROM commerce.orders WHERE id = $1`, orderID).Scan(&s)
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (w *wrWorld) orderStatus(t *testing.T, orderID int64) string {
	t.Helper()
	var s string
	err := w.db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT status FROM commerce.orders WHERE id = $1`, orderID).Scan(&s)
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// requireApperrCode fails unless err is an application error with the given code.
func requireApperrCode(t *testing.T, err error, code string) {
	t.Helper()
	ae, ok := apperr.As(err)
	if !ok || ae.Code != code {
		t.Fatalf("error %v, want application error code %q", err, code)
	}
}
