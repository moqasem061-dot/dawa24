package postgres_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/billing/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Batch 2 (S01): wallet ledger writes and wallet-paid subscriptions must be
// safe under concurrency. These tests run real Postgres with every goroutine
// released at the same moment, and repeat the race in several rounds.

const walletLockRounds = 20

// runAtOnce starts n calls together behind a barrier and returns each result.
func runAtOnce(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = fn(i)
		}(i)
	}
	close(start)
	wg.Wait()
	return errs
}

func firstError(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func countNil(errs []error) int {
	n := 0
	for _, err := range errs {
		if err == nil {
			n++
		}
	}
	return n
}

// walletLedgerSum returns the exact sum of a wallet's ledger rows.
func walletLedgerSum(t *testing.T, db *database.DB, walletID int64) money.Amount {
	t.Helper()
	var sum money.Amount
	err := db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT COALESCE(SUM(amount), 0.00) FROM billing.wallet_transactions WHERE wallet_id = $1;`, walletID).Scan(&sum)
	})
	if err != nil {
		t.Fatalf("ledger sum: %v", err)
	}
	return sum
}

func seedBillingWallet(t *testing.T, repo *postgres.Repository, minor int64) int64 {
	t.Helper()
	ctx := context.Background()
	w, err := repo.GetOrCreateWallet(ctx, testBillingUserID, "EGP")
	if err != nil {
		t.Fatalf("wallet: %v", err)
	}
	if minor > 0 {
		if _, err := repo.RecordTransaction(ctx, w.ID, billing.TxDeposit, money.FromMinor(minor), "test_seed", nil, "seed"); err != nil {
			t.Fatalf("seed deposit: %v", err)
		}
	}
	return w.ID
}

// Ten debits of 100.00 race on a wallet holding exactly 100.00. Exactly one may
// succeed, the balance must end at 0.00 and the ledger must sum to 0.00.
func TestConcurrentWalletDebitsNeverOverdraw(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	repo := postgres.NewRepository(db)
	ctx := context.Background()

	for round := 0; round < walletLockRounds; round++ {
		resetBillingFixtures(t, db)
		walletID := seedBillingWallet(t, repo, 10000)

		errs := runAtOnce(10, func(int) error {
			_, err := repo.RecordTransaction(ctx, walletID, billing.TxWithdrawal, money.FromMinor(-10000), "order", nil, "race debit")
			return err
		})
		if ok := countNil(errs); ok != 1 {
			t.Fatalf("round %d: %d debits of 100.00 succeeded on a 100.00 wallet, want exactly 1", round, ok)
		}
		w, err := repo.GetWallet(ctx, walletID)
		if err != nil {
			t.Fatalf("round %d: get wallet: %v", round, err)
		}
		if w.Balance.Minor() != 0 {
			t.Fatalf("round %d: balance %s, want 0.00", round, w.Balance.String())
		}
		if sum := walletLedgerSum(t, db, walletID); sum.Minor() != 0 {
			t.Fatalf("round %d: ledger sums to %s, want 0.00 (a debit was lost or overdrawn)", round, sum.String())
		}
	}
}

// Deposits and debits interleave on one wallet. Every credit and every debit must
// be recorded once, and the final balance must equal the exact ledger sum.
func TestConcurrentDepositsAndDebitsKeepLedgerExact(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	repo := postgres.NewRepository(db)
	ctx := context.Background()

	for round := 0; round < walletLockRounds; round++ {
		resetBillingFixtures(t, db)
		walletID := seedBillingWallet(t, repo, 0)

		errs := runAtOnce(12, func(i int) error {
			if i%2 == 0 {
				_, err := repo.RecordTransaction(ctx, walletID, billing.TxDeposit, money.FromMinor(2500), "test_seed", nil, "credit")
				return err
			}
			_, err := repo.RecordTransaction(ctx, walletID, billing.TxWithdrawal, money.FromMinor(-1000), "order", nil, "debit")
			return err
		})
		// Debits that land before enough credit exists may be refused; the
		// invariant is that accepted rows and the stored balance agree exactly.
		for i, err := range errs {
			if err != nil && i%2 == 0 {
				t.Fatalf("round %d: a deposit failed: %v", round, err)
			}
		}
		w, err := repo.GetWallet(ctx, walletID)
		if err != nil {
			t.Fatalf("round %d: get wallet: %v", round, err)
		}
		sum := walletLedgerSum(t, db, walletID)
		if sum.Minor() < 0 {
			t.Fatalf("round %d: wallet went negative: ledger sums to %s", round, sum.String())
		}
		if w.Balance.Minor() != sum.Minor() {
			t.Fatalf("round %d: balance projection %s disagrees with ledger sum %s", round, w.Balance.String(), sum.String())
		}
	}
}

// Subscribing with exactly the plan price, eight times at once, must create one
// subscription and record one debit.
func TestConcurrentSubscribeWithExactPriceGivesOneSubscriptionAndOneDebit(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	repo := postgres.NewRepository(db)
	ctx := context.Background()
	svc := billing.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	slug := seedWalletLockPlan(t, db)

	for round := 0; round < walletLockRounds; round++ {
		resetBillingFixtures(t, db)
		walletID := seedBillingWallet(t, repo, 10000)

		errs := runAtOnce(8, func(int) error {
			_, err := svc.SubscribeWithWallet(ctx, testBillingUserID, nil, slug, "monthly", false)
			return err
		})
		if ok := countNil(errs); ok != 1 {
			t.Fatalf("round %d: %d subscribe calls succeeded, want exactly 1; first error: %v", round, ok, firstError(errs))
		}
		if n := countUserSubscriptions(t, db); n != 1 {
			t.Fatalf("round %d: %d subscription rows for the user, want 1", round, n)
		}
		if sum := walletLedgerSum(t, db, walletID); sum.Minor() != 0 {
			t.Fatalf("round %d: ledger sums to %s, want 0.00 (one plan price, one debit)", round, sum.String())
		}
	}
}

// When the wallet debit of a subscription fails, the subscription must not be
// left active. The failure is injected inside the database for this user only.
func TestFailedSubscriptionDebitLeavesNoActiveSubscription(t *testing.T) {
	db := getTestDB(t)
	t.Cleanup(db.Close)
	repo := postgres.NewRepository(db)
	ctx := context.Background()
	svc := billing.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	slug := seedWalletLockPlan(t, db)

	// Cleanups run last-in first-out: the injected failure is removed before the
	// database handle is closed.
	t.Cleanup(func() { removeSubscriptionDebitFailure(t, db) })
	removeSubscriptionDebitFailure(t, db)
	resetBillingFixtures(t, db)
	seedBillingWallet(t, repo, 10000)
	injectSubscriptionDebitFailure(t, db)

	if _, err := svc.SubscribeWithWallet(ctx, testBillingUserID, nil, slug, "monthly", false); err == nil {
		t.Fatal("subscribe succeeded although its wallet debit failed")
	}
	if n := countActiveUserSubscriptions(t, db); n != 0 {
		t.Fatalf("%d active subscriptions remain after a failed wallet debit, want 0", n)
	}
}

// seedWalletLockPlan installs a plan priced at exactly 100.00 a month.
func seedWalletLockPlan(t *testing.T, db *database.DB) string {
	t.Helper()
	const slug = "batch2-wallet-lock"
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO billing.plans (slug, name, price_month, price_year, duration_days, is_active)
			VALUES ($1, '{"ar":"خطة اختبار","en":"Lock test plan"}'::jsonb, 100.00, 1200.00, 30, true)
			ON CONFLICT (slug) DO UPDATE SET price_month = 100.00, price_year = 1200.00, is_active = true;`, slug)
		return err
	})
	if err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	return slug
}

func countUserSubscriptions(t *testing.T, db *database.DB) int {
	t.Helper()
	var n int
	err := db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT count(*) FROM billing.subscriptions WHERE user_id = $1;`, testBillingUserID).Scan(&n)
	})
	if err != nil {
		t.Fatalf("count subscriptions: %v", err)
	}
	return n
}

func countActiveUserSubscriptions(t *testing.T, db *database.DB) int {
	t.Helper()
	var n int
	err := db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT count(*) FROM billing.subscriptions WHERE user_id = $1 AND status = 'active';`, testBillingUserID).Scan(&n)
	})
	if err != nil {
		t.Fatalf("count active subscriptions: %v", err)
	}
	return n
}

func injectSubscriptionDebitFailure(t *testing.T, db *database.DB) {
	t.Helper()
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `
			CREATE OR REPLACE FUNCTION billing.test_fail_subscription_debit() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
			  IF NEW.reference_type = 'subscription_checkout'
			     AND NEW.wallet_id IN (SELECT id FROM billing.wallets WHERE user_id = 88490) THEN
			    RAISE EXCEPTION 'injected failure for the subscription debit';
			  END IF;
			  RETURN NEW;
			END $$;`); err != nil {
			return err
		}
		_, err := tx.Exec(txCtx, `DROP TRIGGER IF EXISTS test_fail_subscription_debit ON billing.wallet_transactions;
			CREATE TRIGGER test_fail_subscription_debit BEFORE INSERT ON billing.wallet_transactions
			FOR EACH ROW EXECUTE FUNCTION billing.test_fail_subscription_debit();`)
		return err
	})
	if err != nil {
		t.Fatalf("inject failure: %v", err)
	}
}

func removeSubscriptionDebitFailure(t *testing.T, db *database.DB) {
	t.Helper()
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `DROP TRIGGER IF EXISTS test_fail_subscription_debit ON billing.wallet_transactions;
			DROP FUNCTION IF EXISTS billing.test_fail_subscription_debit();`)
		return err
	})
	if err != nil {
		t.Fatalf("remove failure injection: %v", err)
	}
}
