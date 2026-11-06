package integration_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Batch 2 (S02, S04, status guard) on the real database. Each supplier's shipment
// is charged when that supplier confirms it, exactly once, in the same
// transaction as the confirmation.

// secondVendor creates another supplier organisation for multi-supplier orders.
func (w *wrWorld) secondVendor(t *testing.T) int64 {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	n := wrSeq.Add(1)
	suffix := fmt.Sprintf("%s-b%d", w.suffix, n)
	var id int64
	if err := w.db.Pool().QueryRow(ctx, `INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ($1, $2, $3, $4, $5, 'vendor', 'approved') RETURNING id`,
		`{"ar":"مورد `+suffix+`"}`, "مورد "+suffix, `{"ar":"مورد `+suffix+`"}`, "TAX-WS-V-"+suffix, "CR-WS-V-"+suffix).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// shipmentOrder creates an unpaid wallet order with one shipment per entry. The
// shipment belongs to the given supplier and has the goods amount in minor units.
// Nothing is charged yet. Negotiation is set when negotiation is true.
func (w *wrWorld) shipmentOrder(t *testing.T, vendors []int64, goodsMinor []int64, statuses []string, orderStatus string, negotiation bool) (int64, []int64) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	n := wrSeq.Add(1)
	var total int64
	for _, g := range goodsMinor {
		total += g
	}
	var orderID int64
	if err := w.db.Pool().QueryRow(ctx, `INSERT INTO commerce.orders (order_number, customer_id, organization_id, status, payment_method, payment_status, subtotal, total_amount, is_negotiation, negotiation_status)
		VALUES ($1, $2, $3, $4, 'wallet', 'unpaid', $5, $5, $6, $7) RETURNING id`,
		fmt.Sprintf("ws-%s-%d", w.suffix, n), w.buyerUser, w.buyerOrg, orderStatus, money.FromMinor(total).String(), negotiation,
		map[bool]string{true: "pending", false: "none"}[negotiation]).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	var shipments []int64
	for i, g := range goodsMinor {
		var id int64
		if err := w.db.Pool().QueryRow(ctx, `INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, total_amount)
			VALUES ($1, $2, $3, $4, $5, $5) RETURNING id`,
			orderID, vendors[i], fmt.Sprintf("ws-%s-%d-%d", w.suffix, n, i), statuses[i], money.FromMinor(g).String()).Scan(&id); err != nil {
			t.Fatal(err)
		}
		shipments = append(shipments, id)
	}
	return orderID, shipments
}

func (w *wrWorld) paymentStatus(t *testing.T, orderID int64) string {
	t.Helper()
	var s string
	err := w.db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT payment_status FROM commerce.orders WHERE id = $1`, orderID).Scan(&s)
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (w *wrWorld) shipmentStatus(t *testing.T, shipmentID int64) string {
	t.Helper()
	var s string
	err := w.db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT status FROM commerce.order_shipments WHERE id = $1`, shipmentID).Scan(&s)
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// debitCount counts the wallet debits recorded for one order.
func (w *wrWorld) debitCount(t *testing.T, orderID int64) int {
	t.Helper()
	var n int
	err := w.db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT count(*) FROM billing.wallet_transactions WHERE wallet_id = $1 AND type = 'withdrawal' AND reference_type = 'order' AND reference_id = $2`, w.walletID, orderID).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (w *wrWorld) confirmAs(vendorOrg int64, shipmentID int64) error {
	ctx := database.WithTenant(context.Background(), vendorOrg)
	_, err := w.commSvc.TransitionShipmentStatusWithWallet(ctx, shipmentID, commerce.StatusConfirmed, nil, "confirmed", w.buyerUser)
	return err
}

func (w *wrWorld) cancelShipmentAs(vendorOrg int64, shipmentID int64) error {
	ctx := database.WithTenant(context.Background(), vendorOrg)
	_, err := w.commSvc.TransitionShipmentStatusWithWallet(ctx, shipmentID, commerce.StatusCancelled, nil, "supplier cancel", 0)
	return err
}

// Two suppliers: each confirmation charges that supplier's shipment once. The
// order is partly paid after the first and paid after the second.
func TestWalletOrderChargesEachSupplierShipmentOnceAndPaysAfterBoth(t *testing.T) {
	w := newWRWorld(t)
	vendorB := w.secondVendor(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg, vendorB}, []int64{50000, 30000}, []string{"pending", "pending"}, "pending", false)

	if err := w.confirmAs(w.vendorOrg, ships[0]); err != nil {
		t.Fatalf("confirm supplier A: %v", err)
	}
	if got := w.balance(t); got != before-50000 {
		t.Fatalf("after supplier A: balance %d, want %d (500.00 for its shipment)", got, before-50000)
	}
	if s := w.paymentStatus(t, orderID); s != "partially_paid" {
		t.Fatalf("after supplier A: payment status %q, want partially_paid", s)
	}
	if err := w.confirmAs(vendorB, ships[1]); err != nil {
		t.Fatalf("confirm supplier B: %v", err)
	}
	if got := w.balance(t); got != before-80000 {
		t.Fatalf("after supplier B: balance %d, want %d (800.00 in total)", got, before-80000)
	}
	if s := w.paymentStatus(t, orderID); s != "paid" {
		t.Fatalf("after both suppliers: payment status %q, want paid", s)
	}
	if n := w.debitCount(t, orderID); n != 2 {
		t.Fatalf("%d wallet debits, want 2 (one per supplier shipment)", n)
	}
}

// Too little balance: the confirmation fails, the shipment stays unconfirmed and
// nothing is debited.
func TestConfirmWithTooLittleBalanceFailsCleanly(t *testing.T) {
	w := newWRWorld(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg}, []int64{before + 50000}, []string{"pending"}, "pending", false)

	if err := w.confirmAs(w.vendorOrg, ships[0]); err == nil {
		t.Fatal("confirm succeeded with less balance than the shipment costs")
	}
	if s := w.shipmentStatus(t, ships[0]); s != "pending" {
		t.Fatalf("shipment status %q after a failed debit, want pending", s)
	}
	if s := w.paymentStatus(t, orderID); s != "unpaid" {
		t.Fatalf("payment status %q after a failed debit, want unpaid", s)
	}
	if n := w.debitCount(t, orderID); n != 0 || w.balance(t) != before {
		t.Fatalf("%d debits and balance %d after a failed confirm, want none and %d", n, w.balance(t), before)
	}
}

// A repeated confirm of the same shipment does not charge again.
func TestRepeatedConfirmDoesNotChargeTwice(t *testing.T) {
	w := newWRWorld(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg}, []int64{50000}, []string{"pending"}, "pending", false)
	if err := w.confirmAs(w.vendorOrg, ships[0]); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	_ = w.confirmAs(w.vendorOrg, ships[0]) // a repeat may be accepted or refused; it must not debit
	if got := w.balance(t); got != before-50000 {
		t.Fatalf("balance %d after a repeated confirm, want %d", got, before-50000)
	}
	if n := w.debitCount(t, orderID); n != 1 {
		t.Fatalf("%d wallet debits after a repeated confirm, want 1", n)
	}
}

// Two confirms of one shipment at once: one debit, 20 rounds.
func TestConcurrentConfirmsOfOneShipmentChargeOnce(t *testing.T) {
	w := newWRWorld(t)
	for round := 0; round < 20; round++ {
		if _, err := w.billRepo.RecordTransaction(context.Background(), w.walletID, billing.TxDeposit, money.FromMinor(100000), "seed", nil, "round seed"); err != nil {
			t.Fatal(err)
		}
		before := w.balance(t)
		orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg}, []int64{10000}, []string{"pending"}, "pending", false)
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		for i := 0; i < 2; i++ {
			go func() {
				defer wg.Done()
				<-start
				_ = w.confirmAs(w.vendorOrg, ships[0])
			}()
		}
		close(start)
		wg.Wait()
		if got := w.balance(t); got != before-10000 {
			t.Fatalf("round %d: balance %d, want %d (one 100.00 debit)", round, got, before-10000)
		}
		if n := w.debitCount(t, orderID); n != 1 {
			t.Fatalf("round %d: %d debits, want 1", round, n)
		}
	}
}

// Supplier B cancels a shipment that was never charged while supplier A's
// shipment is paid: nothing is credited. The buyer then cancels the order and
// gets back exactly A's charge.
func TestCancelAfterOneShipmentChargedRefundsExactlyThatShipment(t *testing.T) {
	w := newWRWorld(t)
	vendorB := w.secondVendor(t)
	start := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg, vendorB}, []int64{50000, 30000}, []string{"pending", "pending"}, "pending", false)
	if err := w.confirmAs(w.vendorOrg, ships[0]); err != nil {
		t.Fatalf("confirm A: %v", err)
	}
	afterA := w.balance(t)
	if err := w.cancelShipmentAs(vendorB, ships[1]); err != nil {
		t.Fatalf("supplier B cancel: %v", err)
	}
	if got := w.balance(t); got != afterA {
		t.Fatalf("supplier B's cancel moved the balance from %d to %d; its shipment was never charged", afterA, got)
	}
	if err := w.apiCancel(orderID); err != nil {
		t.Fatalf("buyer cancel: %v", err)
	}
	if got := w.balance(t); got != start {
		t.Fatalf("after the buyer cancel: balance %d, want %d (exactly A's 500.00 back)", got, start)
	}
	if n := w.refundCredits(t, orderID); n != 1 {
		t.Fatalf("%d refund credits, want 1", n)
	}
}

// A supplier cancels its own charged shipment while another is still open: its
// charge comes back, exactly.
func TestSupplierCancelOfChargedShipmentRefundsItsCharge(t *testing.T) {
	w := newWRWorld(t)
	vendorB := w.secondVendor(t)
	start := w.balance(t)
	_, ships := w.shipmentOrder(t, []int64{w.vendorOrg, vendorB}, []int64{50000, 30000}, []string{"pending", "pending"}, "pending", false)
	if err := w.confirmAs(w.vendorOrg, ships[0]); err != nil {
		t.Fatalf("confirm A: %v", err)
	}
	if err := w.cancelShipmentAs(w.vendorOrg, ships[0]); err != nil {
		t.Fatalf("supplier A cancel: %v", err)
	}
	if got := w.balance(t); got != start {
		t.Fatalf("balance %d after supplier A's cancel, want %d", got, start)
	}
}

// A negotiation can be accepted only while the order is open. A delivered order
// keeps its status, its negotiation and its shipments.
func TestAcceptNegotiationRefusedOnDeliveredOrder(t *testing.T) {
	w := newWRWorld(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg}, []int64{50000}, []string{"delivered"}, "delivered", true)
	err := w.commSvc.AcceptNegotiation(context.Background(), orderID, w.vendorOrg, w.buyerUser, w.buyerUser)
	if err == nil {
		t.Fatal("negotiation accepted on a delivered order")
	}
	if s := w.shipmentStatus(t, ships[0]); s != "delivered" {
		t.Fatalf("shipment status %q, want delivered", s)
	}
	if got := w.balance(t); got != before || w.debitCount(t, orderID) != 0 {
		t.Fatalf("a refused acceptance moved money: balance %d, want %d", got, before)
	}
}

// A supplier that is not on the order cannot accept it. A supplier that is on the
// order accepts only its own shipment, which is charged once.
func TestAcceptNegotiationByWrongPartyChangesNothing(t *testing.T) {
	w := newWRWorld(t)
	vendorB := w.secondVendor(t)
	vendorC := w.secondVendor(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg, vendorB}, []int64{50000, 30000}, []string{"pending", "pending"}, "pending", true)

	if err := w.commSvc.AcceptNegotiation(context.Background(), orderID, vendorC, w.buyerUser, w.buyerUser); err == nil {
		t.Fatal("negotiation accepted by a supplier that is not on the order")
	}
	if s := w.shipmentStatus(t, ships[0]); s != "pending" {
		t.Fatalf("shipment A status %q after a refused party, want pending", s)
	}

	if err := w.commSvc.AcceptNegotiation(context.Background(), orderID, vendorB, w.buyerUser, w.buyerUser); err != nil {
		t.Fatalf("supplier B accept: %v", err)
	}
	if s := w.shipmentStatus(t, ships[1]); s != "confirmed" {
		t.Fatalf("shipment B status %q, want confirmed", s)
	}
	if s := w.shipmentStatus(t, ships[0]); s != "pending" {
		t.Fatalf("supplier B's accept changed supplier A's shipment to %q", s)
	}
	if got := w.balance(t); got != before-30000 {
		t.Fatalf("balance %d after supplier B's accept, want %d (only B's 300.00)", got, before-30000)
	}
}

// A wallet order that was never debited cannot be set to refunded: no money
// would move.
func TestRefundedStatusNeedsARealRefundOnWalletOrders(t *testing.T) {
	w := newWRWorld(t)
	before := w.balance(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg}, []int64{50000}, []string{"delivered"}, "delivered", false)
	ctx := context.Background()

	if err := w.commSvc.TransitionOrderStatus(ctx, orderID, commerce.StatusRefunded, nil, "refund"); err == nil {
		t.Fatal("order set to refunded with no money refunded")
	}
	if s := w.paymentStatus(t, orderID); s != "unpaid" {
		t.Fatalf("payment status %q after a refused refund, want unpaid", s)
	}
	vendorCtx := database.WithTenant(ctx, w.vendorOrg)
	if _, err := w.commSvc.TransitionShipmentStatus(vendorCtx, ships[0], commerce.StatusRefunded, nil, "refund"); err == nil {
		t.Fatal("shipment set to refunded with no money refunded")
	}
	if s := w.shipmentStatus(t, ships[0]); s != "delivered" {
		t.Fatalf("shipment status %q after a refused refund, want delivered", s)
	}
	if w.balance(t) != before {
		t.Fatal("a refused refund moved the balance")
	}
}

// Cash-on-delivery orders are not touched by the wallet rules: no debit, no
// credit, and the status changes still happen.
func TestCODOrdersAreUnaffectedByWalletRules(t *testing.T) {
	w := newWRWorld(t)
	before := w.balance(t)
	var orderID int64
	ctx := database.AsSystem(context.Background())
	if err := w.db.Pool().QueryRow(ctx, `INSERT INTO commerce.orders (order_number, customer_id, organization_id, status, payment_method, payment_status, subtotal, total_amount)
		VALUES ($1, $2, $3, 'pending', 'cod', 'unpaid', 500.00, 500.00) RETURNING id`,
		fmt.Sprintf("cod-%s-%d", w.suffix, time.Now().UnixNano()), w.buyerUser, w.buyerOrg).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	var shipID int64
	if err := w.db.Pool().QueryRow(ctx, `INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, total_amount)
		VALUES ($1, $2, $3, 'pending', 500.00, 500.00) RETURNING id`, orderID, w.vendorOrg, fmt.Sprintf("cod-%s-s", w.suffix)).Scan(&shipID); err != nil {
		t.Fatal(err)
	}
	vendorCtx := database.WithTenant(context.Background(), w.vendorOrg)
	if _, err := w.commSvc.TransitionShipmentStatusWithWallet(vendorCtx, shipID, commerce.StatusConfirmed, nil, "ok", 0); err != nil {
		t.Fatalf("confirm COD shipment: %v", err)
	}
	if s := w.shipmentStatus(t, shipID); s != "confirmed" {
		t.Fatalf("COD shipment status %q, want confirmed", s)
	}
	if err := w.apiCancel(orderID); err != nil {
		t.Fatalf("cancel COD order: %v", err)
	}
	if got := w.balance(t); got != before || w.refundCredits(t, orderID) != 0 {
		t.Fatalf("COD order moved money: balance %d, want %d", got, before)
	}
}

// A cash-on-delivery negotiation is accepted without any wallet debit: the accept
// path charges wallet orders only.
func TestAcceptNegotiationOnCODOrderChargesNothing(t *testing.T) {
	w := newWRWorld(t)
	before := w.balance(t)
	ctx := database.AsSystem(context.Background())
	n := wrSeq.Add(1)
	var orderID int64
	if err := w.db.Pool().QueryRow(ctx, `INSERT INTO commerce.orders (order_number, customer_id, organization_id, status, payment_method, payment_status, subtotal, total_amount, is_negotiation, negotiation_status)
		VALUES ($1, $2, $3, 'pending', 'cod', 'unpaid', 500.00, 500.00, true, 'pending') RETURNING id`,
		fmt.Sprintf("cn-%s-%d", w.suffix, n), w.buyerUser, w.buyerOrg).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	var shipID int64
	if err := w.db.Pool().QueryRow(ctx, `INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, total_amount)
		VALUES ($1, $2, $3, 'pending', 500.00, 500.00) RETURNING id`, orderID, w.vendorOrg, fmt.Sprintf("cn-%s-%d-s", w.suffix, n)).Scan(&shipID); err != nil {
		t.Fatal(err)
	}
	if err := w.commSvc.AcceptNegotiation(context.Background(), orderID, w.vendorOrg, w.buyerUser, 0); err != nil {
		t.Fatalf("accept on a COD negotiation: %v", err)
	}
	if s := w.shipmentStatus(t, shipID); s != "confirmed" {
		t.Fatalf("shipment status %q after accept, want confirmed", s)
	}
	if got := w.balance(t); got != before || w.debitCount(t, orderID) != 0 {
		t.Fatalf("COD negotiation moved money: balance %d, want %d", got, before)
	}
}
