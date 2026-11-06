package integration_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	billingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/billing/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	promoPostgres "github.com/moqasem061-dot/dawa24/internal/modules/promo/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Batch 2 (S24) on the real database: a sponsorship request and its credit, and a
// paid package and its wallet debit, are each written in one transaction.

var spSeq atomic.Int64

type spWorld struct {
	db       *database.DB
	svc      *promo.Service
	billRepo *billingPostgres.Repository
	org      int64
	user     int64
	pkg      int64
}

func newSPWorld(t *testing.T, price string, walletMinor int64) *spWorld {
	t.Helper()
	db := wrOpenDB(t)
	ctx := database.AsSystem(context.Background())
	n := spSeq.Add(1)
	suffix := fmt.Sprintf("%d-%d", time.Now().UnixNano()%1000000000, n)
	w := &spWorld{db: db, billRepo: billingPostgres.NewRepository(db)}
	if err := db.Pool().QueryRow(ctx, `INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ($1, $2, $3, $4, $5, 'vendor', 'approved') RETURNING id`,
		`{"ar":"مورد `+suffix+`"}`, "مورد "+suffix, `{"ar":"مورد `+suffix+`"}`, "TAX-SP-"+suffix, "CR-SP-"+suffix).Scan(&w.org); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool().QueryRow(ctx, `INSERT INTO identity.users (name, email, password_hash, role, status)
		VALUES ($1, $2, '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active') RETURNING id`,
		`{"ar":"مستخدم `+suffix+`"}`, "sp-"+suffix+"@dawa24.test").Scan(&w.user); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool().QueryRow(ctx, `INSERT INTO promo.offer_packages (name, price, duration_days, is_active, credits)
		VALUES ($1, $2, 30, true, 5) RETURNING id`, `{"ar":"باقة `+suffix+`"}`, price).Scan(&w.pkg); err != nil {
		t.Fatal(err)
	}
	wallet, err := w.billRepo.GetOrCreateWallet(ctx, w.user, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	if walletMinor > 0 {
		if _, err := w.billRepo.RecordTransaction(ctx, wallet.ID, billing.TxDeposit, money.FromMinor(walletMinor), "seed", nil, "seed deposit"); err != nil {
			t.Fatal(err)
		}
	}
	w.svc = promo.NewService(promoPostgres.NewRepository(db), slog.New(slog.NewTextHandler(io.Discard, nil)))
	w.svc.SetWalletDebiter(func(ctx context.Context, tx pgx.Tx, orgID int64, amount money.Amount, description string) (*int64, error) {
		id, err := w.billRepo.WithdrawInTx(ctx, tx, w.user, amount, "sponsorship_package", nil, description)
		if err != nil {
			return nil, err
		}
		return &id, nil
	})
	return w
}

func (w *spWorld) tenant() context.Context {
	return database.WithTenant(context.Background(), w.org)
}

// purchase inserts an active purchase of the given credits and returns its id.
func (w *spWorld) purchase(t *testing.T, credits int) int64 {
	t.Helper()
	var id int64
	if err := w.db.Pool().QueryRow(database.AsSystem(context.Background()), `
		INSERT INTO promo.sponsorship_purchases (organization_id, package_id, credits_total, credits_used, starts_at, expires_at, status, amount, source_system)
		VALUES ($1, $2, $3, 0, now(), now() + interval '30 days', 'active', 0.00, 'test')
		RETURNING id`, w.org, w.pkg, credits).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (w *spWorld) scalar(t *testing.T, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	err := w.db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, sql, args...).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (w *spWorld) balance(t *testing.T) int64 {
	t.Helper()
	var bal money.Amount
	err := w.db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT COALESCE((SELECT balance_after FROM billing.wallet_transactions t JOIN billing.wallets wl ON wl.id = t.wallet_id WHERE wl.user_id = $1 AND wl.currency = 'EGP' ORDER BY t.id DESC LIMIT 1), 0.00)`, w.user).Scan(&bal)
	})
	if err != nil {
		t.Fatal(err)
	}
	return bal.Minor()
}

// spRunAtOnce starts n calls together behind a barrier and returns each result.
func spRunAtOnce(n int, fn func(i int) error) []error {
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

// Eight vendors' requests race for three credits. Exactly three succeed, and
// exactly three requests stay pending: none is left without its credit. 20 rounds.
func TestConcurrentSponsorshipRequestsMatchCreditsExactly(t *testing.T) {
	w := newSPWorld(t, "0.00", 0)
	for round := 0; round < 20; round++ {
		purchaseID := w.purchase(t, 3)
		errs := spRunAtOnce(8, func(i int) error {
			_, err := w.svc.SubmitSponsorshipRequest(w.tenant(), promo.SponsorItemProduct, int64(100000*round+i+1), w.pkg)
			return err
		})
		ok := 0
		for _, err := range errs {
			if err == nil {
				ok++
			}
		}
		if ok != 3 {
			t.Fatalf("round %d: %d requests succeeded for 3 credits, want exactly 3", round, ok)
		}
		if n := w.scalar(t, `SELECT count(*) FROM promo.sponsorship_requests WHERE purchase_id = $1`, purchaseID); n != 3 {
			t.Fatalf("round %d: %d requests exist, want 3 (a request without its credit was left behind)", round, n)
		}
		if used := w.scalar(t, `SELECT credits_used FROM promo.sponsorship_purchases WHERE id = $1`, purchaseID); used != 3 {
			t.Fatalf("round %d: credits used %d, want 3", round, used)
		}
	}
}

// A paid package is charged and recorded together: the wallet goes down by the
// price and one purchase exists, linked to the debit.
func TestPaidPackagePurchaseChargesAndRecordsTogether(t *testing.T) {
	w := newSPWorld(t, "100.00", 10000)
	if _, err := w.svc.PurchaseSponsorshipPackage(w.tenant(), w.pkg, false, "monthly"); err != nil {
		t.Fatalf("purchase: %v", err)
	}
	if got := w.balance(t); got != 0 {
		t.Fatalf("balance %d after a 100.00 package from a 100.00 wallet, want 0", got)
	}
	if n := w.scalar(t, `SELECT count(*) FROM promo.sponsorship_purchases WHERE organization_id = $1`, w.org); n != 1 {
		t.Fatalf("%d purchases, want 1", n)
	}
	if n := w.scalar(t, `SELECT count(*) FROM promo.sponsorship_purchases WHERE organization_id = $1 AND source_id IS NOT NULL`, w.org); n != 1 {
		t.Fatalf("%d purchases linked to a wallet debit, want 1", n)
	}
}

// When the purchase row cannot be written, the wallet debit is rolled back too.
// The failure is injected for this organisation only.
func TestFailedPackagePurchaseLeavesNoDebit(t *testing.T) {
	w := newSPWorld(t, "100.00", 10000)
	ctx := database.AsSystem(context.Background())
	fn := fmt.Sprintf(`CREATE OR REPLACE FUNCTION promo.test_fail_purchase_%d() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.organization_id = %d THEN RAISE EXCEPTION 'injected failure for the purchase row'; END IF; RETURN NEW; END $$;`, w.org, w.org)
	trigger := fmt.Sprintf(`CREATE TRIGGER test_fail_purchase_%d BEFORE INSERT ON promo.sponsorship_purchases
		FOR EACH ROW EXECUTE FUNCTION promo.test_fail_purchase_%d();`, w.org, w.org)
	if err := w.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, fn); err != nil {
			return err
		}
		_, err := tx.Exec(txCtx, trigger)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = w.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(txCtx, fmt.Sprintf(`DROP TRIGGER IF EXISTS test_fail_purchase_%d ON promo.sponsorship_purchases;
				DROP FUNCTION IF EXISTS promo.test_fail_purchase_%d();`, w.org, w.org))
			return err
		})
	})

	before := w.balance(t)
	if _, err := w.svc.PurchaseSponsorshipPackage(w.tenant(), w.pkg, false, "monthly"); err == nil {
		t.Fatal("purchase succeeded although its row could not be written")
	}
	if got := w.balance(t); got != before {
		t.Fatalf("balance %d after a failed purchase, want %d: the debit was not rolled back", got, before)
	}
	if n := w.scalar(t, `SELECT count(*) FROM promo.sponsorship_purchases WHERE organization_id = $1`, w.org); n != 0 {
		t.Fatalf("%d purchases after a failed purchase, want 0", n)
	}
}

// A wallet that cannot pay for a paid package gets no purchase.
func TestUnaffordablePackageGetsNoPurchase(t *testing.T) {
	w := newSPWorld(t, "100.00", 5000)
	if _, err := w.svc.PurchaseSponsorshipPackage(w.tenant(), w.pkg, false, "monthly"); err == nil {
		t.Fatal("purchase succeeded with 50.00 in the wallet for a 100.00 package")
	}
	if got := w.balance(t); got != 5000 {
		t.Fatalf("balance %d after a refused purchase, want 5000", got)
	}
	if n := w.scalar(t, `SELECT count(*) FROM promo.sponsorship_purchases WHERE organization_id = $1`, w.org); n != 0 {
		t.Fatalf("%d purchases after a refused purchase, want 0", n)
	}
}
