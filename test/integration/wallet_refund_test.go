package integration_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	billingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/billing/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	commercePostgres "github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Batch 1B (S03) on the real database: the commerce service, the wallet ledger
// and the billing admin refund, wired the way the composition root wires them.

var wrSeq atomic.Int64

// wrFixtureID returns a fresh id in microseconds since the epoch plus a counter.
// It sits far above the database sequence and the fixed ids that other tests
// insert, so a fixture row cannot collide with either.
func wrFixtureID() int64 {
	return time.Now().UnixNano()/1000 + wrSeq.Add(1)
}

// wrOpenDB connects with the configured URL. Unlike getTestDB it does not skip
// a superuser connection: the sandbox runs as one, and these tests check money.
func wrOpenDB(t *testing.T) *database.DB {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL or TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := database.Open(ctx, config.Database{URL: dbURL, MaxConns: 5, MinConns: 1, MaxConnLifetime: time.Hour, MaxConnIdleTime: 30 * time.Minute, StatementTimeout: 10 * time.Second})
	if err != nil {
		t.Skipf("cannot connect to database: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

type wrWorld struct {
	db        *database.DB
	suffix    string
	commSvc   *commerce.Service
	billRepo  *billingPostgres.Repository
	buyerUser int64
	buyerOrg  int64
	vendorOrg int64
	walletID  int64
}

func newWRWorld(t *testing.T) *wrWorld {
	t.Helper()
	db := wrOpenDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := database.AsSystem(context.Background())
	n := wrSeq.Add(1)
	suffix := fmt.Sprintf("%d-%d", time.Now().UnixNano()%1000000000, n)
	w := &wrWorld{db: db, billRepo: billingPostgres.NewRepository(db), suffix: fmt.Sprintf("%d", time.Now().UnixNano()%1000000000)}
	w.commSvc = commerce.NewService(commercePostgres.NewRepository(db).WithWalletRefunder(w.billRepo).WithWalletPayer(w.billRepo), logger)
	w.buyerOrg, w.vendorOrg, w.buyerUser = wrFixtureID(), wrFixtureID(), wrFixtureID()
	if err := db.Pool().QueryRow(ctx, `INSERT INTO org.organizations (id, name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'customer', 'approved') RETURNING id`,
		w.buyerOrg, `{"ar":"صيدلية `+suffix+`"}`, "صيدلية "+suffix, `{"ar":"صيدلية `+suffix+`"}`, "TAX-WR-C-"+suffix, "CR-WR-C-"+suffix).Scan(&w.buyerOrg); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool().QueryRow(ctx, `INSERT INTO org.organizations (id, name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'vendor', 'approved') RETURNING id`,
		w.vendorOrg, `{"ar":"مورد `+suffix+`"}`, "مورد "+suffix, `{"ar":"مورد `+suffix+`"}`, "TAX-WR-V-"+suffix, "CR-WR-V-"+suffix).Scan(&w.vendorOrg); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool().QueryRow(ctx, `INSERT INTO identity.users (id, name, email, password_hash, role, status)
		VALUES ($1, $2, $3, '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active') RETURNING id`,
		w.buyerUser, `{"ar":"مستخدم `+suffix+`"}`, "wr-"+suffix+"@dawa24.test").Scan(&w.buyerUser); err != nil {
		t.Fatal(err)
	}
	wallet, err := w.billRepo.GetOrCreateWallet(ctx, w.buyerUser, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	w.walletID = wallet.ID
	if _, err := w.billRepo.RecordTransaction(ctx, w.walletID, billing.TxDeposit, money.FromMinor(100000), "seed", nil, "seed deposit"); err != nil {
		t.Fatal(err)
	}
	return w
}

// order creates a wallet (or COD) order with one shipment per status. A
// debitMinor above zero records the wallet debit for the order.
func (w *wrWorld) order(t *testing.T, method string, debitMinor int64, shipmentStatuses ...string) (int64, []int64) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	n := wrSeq.Add(1)
	var orderID int64
	if err := w.db.Pool().QueryRow(ctx, `INSERT INTO commerce.orders (order_number, customer_id, organization_id, status, payment_method, payment_status, subtotal, total_amount)
		VALUES ($1, $2, $3, 'pending', $4, $5, 1000.00, 1000.00) RETURNING id`,
		fmt.Sprintf("wr-%s-%d", w.suffix, n), w.buyerUser, w.buyerOrg, method, map[bool]string{true: "paid", false: "unpaid"}[debitMinor > 0]).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	var shipments []int64
	for i, s := range shipmentStatuses {
		var id int64
		if err := w.db.Pool().QueryRow(ctx, `INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, total_amount)
			VALUES ($1, $2, $3, $4, 500.00, 500.00) RETURNING id`, orderID, w.vendorOrg, fmt.Sprintf("wr-%s-%d-%d", w.suffix, n, i), s).Scan(&id); err != nil {
			t.Fatal(err)
		}
		shipments = append(shipments, id)
	}
	if debitMinor > 0 {
		if _, err := w.billRepo.RecordTransaction(ctx, w.walletID, billing.TxWithdrawal, money.FromMinor(-debitMinor), "order", &orderID, "debit"); err != nil {
			t.Fatal(err)
		}
	}
	return orderID, shipments
}

func (w *wrWorld) balance(t *testing.T) int64 {
	t.Helper()
	var bal money.Amount
	err := w.db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT balance_after FROM billing.wallet_transactions WHERE wallet_id=$1 ORDER BY id DESC LIMIT 1`, w.walletID).Scan(&bal)
	})
	if err != nil {
		t.Fatal(err)
	}
	return bal.Minor()
}

func (w *wrWorld) refundCredits(t *testing.T, orderID int64) int {
	t.Helper()
	var n int
	err := w.db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT count(*) FROM billing.wallet_transactions WHERE wallet_id=$1 AND reference_type='refund' AND reference_id=$2`, w.walletID, orderID).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (w *wrWorld) apiCancel(orderID int64) error {
	return w.commSvc.CancelAPIOrder(context.Background(), commerce.APICancelRequest{
		OrderID: orderID, UserID: w.buyerUser, OrganizationID: w.buyerOrg,
	})
}

// Two API cancels at once: one succeeds and the wallet is credited once. 20 rounds.
func TestWalletOrderAPICancelCreditsOnceUnderRace(t *testing.T) {
	w := newWRWorld(t)
	for i := 0; i < 20; i++ {
		orderID, _ := w.order(t, "wallet", 100000, "pending")
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for j := 0; j < 2; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results <- w.apiCancel(orderID)
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("round %d: %d cancels succeeded, want 1", i, successes)
		}
		if n := w.refundCredits(t, orderID); n != 1 {
			t.Fatalf("round %d: %d refund credits, want 1", i, n)
		}
		if bal := w.balance(t); bal != 100000 {
			t.Fatalf("round %d: balance %d, want 100000 (debit returned once)", i, bal)
		}
	}
}

// A repeated cancel of a cancelled wallet order is refused and credits nothing.
func TestWalletOrderRepeatedAPICancelIsRefused(t *testing.T) {
	w := newWRWorld(t)
	orderID, _ := w.order(t, "wallet", 100000, "pending")
	before := w.balance(t)
	if err := w.apiCancel(orderID); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	after := w.balance(t)
	if after != before+100000 {
		t.Fatalf("balance %d after cancel, want %d (debit returned once)", after, before+100000)
	}
	if err := w.apiCancel(orderID); err == nil {
		t.Fatal("second cancel of a cancelled wallet order was accepted")
	}
	if n := w.refundCredits(t, orderID); n != 1 || w.balance(t) != after {
		t.Fatalf("refund credits %d, balance %d: a repeated cancel must not credit again", n, w.balance(t))
	}
}

// A multi-vendor order charged 400.00 is refunded 400.00 only, not its 1000.00 total.
func TestPartlyDebitedWalletOrderRefundsOnlyWhatWasCharged(t *testing.T) {
	w := newWRWorld(t)
	orderID, _ := w.order(t, "wallet", 40000, "pending", "pending")
	before := w.balance(t)
	if err := w.apiCancel(orderID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got := w.balance(t); got != before+40000 {
		t.Fatalf("balance %d, want %d: refund must equal the 400.00 debit", got, before+40000)
	}
}

// An unpaid wallet order creates no credit when it is cancelled.
func TestUnpaidWalletOrderCreatesNoCredit(t *testing.T) {
	w := newWRWorld(t)
	orderID, _ := w.order(t, "wallet", 0, "pending")
	before := w.balance(t)
	if err := w.apiCancel(orderID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if n := w.refundCredits(t, orderID); n != 0 || w.balance(t) != before {
		t.Fatalf("unpaid order credited %d times, balance %d (before %d)", n, w.balance(t), before)
	}
}

// A buyer cancel races an admin finance refund of the same order debit. The
// debit is returned exactly once, whichever runs first.
func TestBuyerCancelRacesAdminRefundOfDebit(t *testing.T) {
	w := newWRWorld(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	billSvc := billing.NewService(w.billRepo, logger)
	for i := 0; i < 20; i++ {
		orderID, _ := w.order(t, "wallet", 100000, "pending")
		var debitTxID int64
		err := w.db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(txCtx, `SELECT id FROM billing.wallet_transactions WHERE wallet_id=$1 AND reference_type='order' AND reference_id=$2`, w.walletID, orderID).Scan(&debitTxID)
		})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_ = w.apiCancel(orderID)
		}()
		go func() {
			defer wg.Done()
			<-start
			_, _ = billSvc.AdminRefundTransaction(context.Background(), debitTxID, "admin refund", 1)
		}()
		close(start)
		wg.Wait()
		if bal := w.balance(t); bal != 100000 {
			t.Fatalf("round %d: balance %d, want 100000: the 1000.00 debit must be returned exactly once", i, bal)
		}
	}
}

// Batch 2 (S02): a legacy wallet debit (one debit for the order, made before
// per-supplier payments, with no shipment charge on record) cannot be partly
// cancelled. The debit may belong to the shipment being cancelled, so nothing is
// credited until an administrator reviews the order.
func TestSupplierPartialCancelOfLegacyDebitIsRefused(t *testing.T) {
	w := newWRWorld(t)
	orderID, shipments := w.order(t, "wallet", 100000, "pending", "pending")
	before := w.balance(t)
	ctx := database.WithTenant(context.Background(), w.vendorOrg)
	if _, err := w.commSvc.TransitionShipmentStatus(ctx, shipments[0], commerce.StatusCancelled, nil, "supplier cancel"); err == nil {
		t.Fatal("partial cancel of a legacy wallet debit was accepted")
	}
	if got := w.balance(t); got != before || w.refundCredits(t, orderID) != 0 {
		t.Fatalf("refused partial cancel moved money: balance %d, want %d", got, before)
	}
}

// The admin route to cancel a wallet order refunds it; the admin refund route
// returns a delivered wallet order's debit once.
func TestAdminCancelAndRefundOfWalletOrder(t *testing.T) {
	w := newWRWorld(t)
	ctx := context.Background()
	adminID := w.buyerUser
	cancelID, _ := w.order(t, "wallet", 100000, "pending")
	before := w.balance(t)
	if err := w.commSvc.TransitionOrderStatus(ctx, cancelID, commerce.StatusCancelled, &adminID, "admin cancel"); err != nil {
		t.Fatalf("admin cancel: %v", err)
	}
	if got := w.balance(t); got != before+100000 {
		t.Fatalf("admin cancel: balance %d, want %d", got, before+100000)
	}
	refundID, _ := w.order(t, "wallet", 100000, "delivered")
	if err := w.db.Pool().QueryRow(database.AsSystem(ctx), `UPDATE commerce.orders SET status='delivered' WHERE id=$1 RETURNING id`, refundID).Scan(new(int64)); err != nil {
		t.Fatal(err)
	}
	before = w.balance(t)
	if err := w.commSvc.TransitionOrderStatus(ctx, refundID, commerce.StatusRefunded, &adminID, "admin refund"); err != nil {
		t.Fatalf("admin refund: %v", err)
	}
	if got := w.balance(t); got != before+100000 {
		t.Fatalf("admin refund: balance %d, want %d", got, before+100000)
	}
	if err := w.commSvc.TransitionOrderStatus(ctx, refundID, commerce.StatusRefunded, &adminID, "again"); err != nil {
		t.Fatalf("repeat admin refund should change nothing and succeed: %v", err)
	}
	if n := w.refundCredits(t, refundID); n != 1 {
		t.Fatalf("%d refund credits after repeated admin refund, want 1", n)
	}
}
