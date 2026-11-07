package ui_test

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	billingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/billing/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Website buyer cancellation of wallet-paid orders (Batch 1B, S03). Every test
// uses the real database, the real wallet ledger and the real HTTP route.

var websiteCancelSeq atomic.Int64

// websiteFixtureID returns a fresh id in microseconds since the epoch plus a
// counter. It sits far above the database sequence and the fixed ids that other
// tests insert, so a fixture row cannot collide with either.
func websiteFixtureID() int64 {
	return time.Now().UnixNano()/1000 + websiteCancelSeq.Add(1)
}

type websiteCancelFixture struct {
	actor    authctx.Actor
	walletID int64
	orderID  int64
}

// seedWebsiteCancelOrder creates a buyer with a 1000.00 deposit and a pending
// order. debitMinor is the wallet debit recorded for the order (0 for an unpaid
// order); totalMinor is the order total.
func seedWebsiteCancelOrder(t *testing.T, db *database.DB, method string, debitMinor, totalMinor int64) websiteCancelFixture {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	n := websiteCancelSeq.Add(1)
	suffix := fmt.Sprintf("%d-%d", time.Now().UnixNano()%1000000000, n)
	customerOrgID, vendorOrgID, userID := websiteFixtureID(), websiteFixtureID(), websiteFixtureID()
	if err := db.Pool().QueryRow(ctx, `INSERT INTO org.organizations (id, name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'customer', 'approved') RETURNING id`,
		customerOrgID, `{"ar":"صيدلية `+suffix+`"}`, "صيدلية "+suffix, `{"ar":"صيدلية `+suffix+`"}`, "TAX-C-"+suffix, "CR-C-"+suffix).Scan(&customerOrgID); err != nil {
		t.Fatalf("customer org: %v", err)
	}
	if err := db.Pool().QueryRow(ctx, `INSERT INTO org.organizations (id, name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'vendor', 'approved') RETURNING id`,
		vendorOrgID, `{"ar":"مورد `+suffix+`"}`, "مورد "+suffix, `{"ar":"مورد `+suffix+`"}`, "TAX-V-"+suffix, "CR-V-"+suffix).Scan(&vendorOrgID); err != nil {
		t.Fatalf("vendor org: %v", err)
	}
	if err := db.Pool().QueryRow(ctx, `INSERT INTO identity.users (id, name, email, password_hash, role, status)
		VALUES ($1, $2, $3, '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active') RETURNING id`,
		userID, `{"ar":"مستخدم `+suffix+`"}`, "wc-"+suffix+"@dawa24.test").Scan(&userID); err != nil {
		t.Fatalf("user: %v", err)
	}
	repo := billingPostgres.NewRepository(db)
	wallet, err := repo.GetOrCreateWallet(ctx, userID, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RecordTransaction(ctx, wallet.ID, billing.TxDeposit, money.FromMinor(100000), "seed", nil, "seed deposit"); err != nil {
		t.Fatal(err)
	}
	payment := "unpaid"
	if debitMinor > 0 {
		payment = "paid"
	}
	total := money.FromMinor(totalMinor).String()
	var orderID int64
	if err := db.Pool().QueryRow(ctx, `INSERT INTO commerce.orders (order_number, customer_id, organization_id, status, payment_method, payment_status, subtotal, total_amount)
		VALUES ($1, $2, $3, 'pending', $4, $5, $6, $6) RETURNING id`,
		"wc-"+suffix, userID, customerOrgID, method, payment, total).Scan(&orderID); err != nil {
		t.Fatalf("order: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, total_amount)
		VALUES ($1, $2, $3, 'pending', $4, $4)`, orderID, vendorOrgID, "wc-s-"+suffix, total); err != nil {
		t.Fatalf("shipment: %v", err)
	}
	if debitMinor > 0 {
		if _, err := repo.RecordTransaction(ctx, wallet.ID, billing.TxWithdrawal, money.FromMinor(-debitMinor), "order", &orderID, "debit for order"); err != nil {
			t.Fatal(err)
		}
	}
	return websiteCancelFixture{
		actor: authctx.Actor{
			UserID: userID, OrganizationID: customerOrgID, OrgType: "customer", Scope: "pharmacy",
			Permissions: []string{"pharmacy.order.update", "order.update", "vendor.buying.order.update"},
		},
		walletID: wallet.ID,
		orderID:  orderID,
	}
}

func websiteWalletBalanceMinor(t *testing.T, db *database.DB, walletID int64) int64 {
	t.Helper()
	var bal money.Amount
	err := db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT balance_after FROM billing.wallet_transactions WHERE wallet_id=$1 ORDER BY id DESC LIMIT 1`, walletID).Scan(&bal)
	})
	if err != nil {
		t.Fatal(err)
	}
	return bal.Minor()
}

func websiteRefundRows(t *testing.T, db *database.DB, walletID, orderID int64) int {
	t.Helper()
	var n int
	err := db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT count(*) FROM billing.wallet_transactions WHERE wallet_id=$1 AND reference_type='refund' AND reference_id=$2`, walletID, orderID).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func websiteOrderStatus(t *testing.T, db *database.DB, orderID int64) string {
	t.Helper()
	var status string
	err := db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT status FROM commerce.orders WHERE id=$1`, orderID).Scan(&status)
	})
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// Two buyer cancels at the same moment: the wallet is credited once.
func TestWebsiteWalletCancelCreditsOnceUnderRace(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}
	h := newRealUIHandler(t, db)
	for i := 0; i < 20; i++ {
		fx := seedWebsiteCancelOrder(t, db, "wallet", 100000, 100000)
		path := fmt.Sprintf("/orders/%d/cancel", fx.orderID)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for j := 0; j < 2; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				doPOST(t, h, path, url.Values{"reason": {"buyer changed mind"}}, fx.actor)
			}()
		}
		close(start)
		wg.Wait()
		if got := websiteOrderStatus(t, db, fx.orderID); got != "cancelled" {
			t.Fatalf("round %d: order status %q, want cancelled", i, got)
		}
		if n := websiteRefundRows(t, db, fx.walletID, fx.orderID); n != 1 {
			t.Fatalf("round %d: %d refund credits, want exactly 1", i, n)
		}
		if bal := websiteWalletBalanceMinor(t, db, fx.walletID); bal != 100000 {
			t.Fatalf("round %d: wallet balance %d minor, want 100000 (deposit restored exactly once)", i, bal)
		}
	}
}

// A repeated cancel of an order the buyer already cancelled credits nothing more.
func TestWebsiteRepeatedWalletCancelCreditsOnce(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}
	h := newRealUIHandler(t, db)
	fx := seedWebsiteCancelOrder(t, db, "wallet", 100000, 100000)
	path := fmt.Sprintf("/orders/%d/cancel", fx.orderID)
	for i := 0; i < 2; i++ {
		doPOST(t, h, path, url.Values{"reason": {"again"}}, fx.actor)
	}
	if n := websiteRefundRows(t, db, fx.walletID, fx.orderID); n != 1 {
		t.Fatalf("%d refund credits after two cancels, want 1", n)
	}
	if bal := websiteWalletBalanceMinor(t, db, fx.walletID); bal != 100000 {
		t.Fatalf("wallet balance %d minor, want 100000", bal)
	}
}

// A multi-vendor order charged 400.00 is refunded 400.00, not its 1000.00 total.
func TestWebsitePartlyDebitedWalletOrderRefundsOnlyWhatWasCharged(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}
	h := newRealUIHandler(t, db)
	fx := seedWebsiteCancelOrder(t, db, "wallet", 40000, 100000)
	doPOST(t, h, fmt.Sprintf("/orders/%d/cancel", fx.orderID), url.Values{"reason": {"cancel"}}, fx.actor)
	if got := websiteOrderStatus(t, db, fx.orderID); got != "cancelled" {
		t.Fatalf("order status %q, want cancelled", got)
	}
	if bal := websiteWalletBalanceMinor(t, db, fx.walletID); bal != 100000 {
		t.Fatalf("wallet balance %d minor, want 100000: refund must equal the 400.00 debit", bal)
	}
}

// An unpaid wallet order creates no credit when it is cancelled.
func TestWebsiteUnpaidWalletOrderCreatesNoCredit(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}
	h := newRealUIHandler(t, db)
	fx := seedWebsiteCancelOrder(t, db, "wallet", 0, 50000)
	doPOST(t, h, fmt.Sprintf("/orders/%d/cancel", fx.orderID), url.Values{"reason": {"cancel"}}, fx.actor)
	if got := websiteOrderStatus(t, db, fx.orderID); got != "cancelled" {
		t.Fatalf("order status %q, want cancelled", got)
	}
	if n := websiteRefundRows(t, db, fx.walletID, fx.orderID); n != 0 {
		t.Fatalf("%d refund credits for an unpaid order, want 0", n)
	}
	if bal := websiteWalletBalanceMinor(t, db, fx.walletID); bal != 100000 {
		t.Fatalf("wallet balance %d minor, want 100000 (unchanged)", bal)
	}
}

// Cash-on-delivery orders still cancel on the website and touch no wallet.
func TestWebsiteCODCancelStillWorks(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}
	h := newRealUIHandler(t, db)
	fx := seedWebsiteCancelOrder(t, db, "cod", 0, 50000)
	doPOST(t, h, fmt.Sprintf("/orders/%d/cancel", fx.orderID), url.Values{"reason": {"cancel"}}, fx.actor)
	if got := websiteOrderStatus(t, db, fx.orderID); got != "cancelled" {
		t.Fatalf("order status %q, want cancelled", got)
	}
	if n := websiteRefundRows(t, db, fx.walletID, fx.orderID); n != 0 {
		t.Fatalf("%d refund credits for a COD order, want 0", n)
	}
}
