package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/billing/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func getTestDB(t *testing.T) *database.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database integration test in short mode")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config.Database{
		URL:              dbURL,
		MaxConns:         5,
		MinConns:         1,
		MaxConnLifetime:  time.Hour,
		MaxConnIdleTime:  30 * time.Minute,
		StatementTimeout: 10 * time.Second,
	}

	db, err := database.Open(ctx, cfg)
	if err != nil {
		t.Skipf("cannot connect to database: %v; skipping", err)
	}

	migrations, _ := database.LoadMigrations(dbfs.Migrations, "migrations")
	if pending, _ := db.PendingCount(ctx, migrations); pending > 0 {
		t.Fatalf("%d migrations pending", pending)
	}

	return db
}

const (
	testBillingUserID int64 = 88490
	testBillingOrgID  int64 = 88491
)

func resetBillingFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM billing.user_payment_methods WHERE user_id = $1`, testBillingUserID); err != nil {
			return fmt.Errorf("delete user_payment_methods: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM billing.wallet_transactions WHERE wallet_id IN (SELECT id FROM billing.wallets WHERE user_id = $1)`, testBillingUserID); err != nil {
			return fmt.Errorf("delete wallet_transactions: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM billing.wallets WHERE user_id = $1`, testBillingUserID); err != nil {
			return fmt.Errorf("delete wallets: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM billing.payments WHERE user_id = $1`, testBillingUserID); err != nil {
			return fmt.Errorf("delete payments: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM billing.subscriptions WHERE user_id = $1`, testBillingUserID); err != nil {
			return fmt.Errorf("delete subscriptions: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM billing.invoices WHERE organization_id = $1`, testBillingOrgID); err != nil {
			return fmt.Errorf("delete invoices: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id = $1`, testBillingOrgID); err != nil {
			return fmt.Errorf("delete organizations: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = $1`, testBillingUserID); err != nil {
			return fmt.Errorf("delete users: %w", err)
		}

		if _, err := tx.Exec(txCtx, `INSERT INTO identity.users (id, email, password_hash, name)
			VALUES ($1, 'user88490@example.com', 'x', '{"ar":"مستخدم","en":"Billing User"}'::jsonb)
			ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email`, testBillingUserID); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"ar":"مؤسسة الفواتير","en":"Billing Org"}'::jsonb)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testBillingOrgID); err != nil {
			return fmt.Errorf("insert org: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures failed: %v", err)
	}
}

func TestBillingRepository(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	resetBillingFixtures(t, db)

	ctx := context.Background()
	repo := postgres.NewRepository(db)

	t.Run("Wallet_Operations", func(t *testing.T) {
		w, err := repo.GetOrCreateWallet(ctx, testBillingUserID, "EGP")
		if err != nil {
			t.Fatalf("GetOrCreateWallet failed: %v", err)
		}

		if w.Balance.Minor() != 0 {
			t.Errorf("expected 0 balance, got %v", w.Balance)
		}

		amount := money.MustParse("150.00")
		txRec, err := repo.RecordTransaction(ctx, w.ID, billing.TxDeposit, amount, "manual", nil, "test deposit")
		if err != nil {
			t.Fatalf("RecordTransaction failed: %v", err)
		}

		if txRec.Amount.Minor() != amount.Minor() {
			t.Errorf("money round-trip failed: got %v, want %v", txRec.Amount, amount)
		}
		if txRec.ReferenceID != nil {
			t.Errorf("expected nil ReferenceID, got %v", txRec.ReferenceID)
		}

		w2, err := repo.GetWallet(ctx, w.ID)
		if err != nil {
			t.Fatalf("GetWallet failed: %v", err)
		}
		if w2.Balance.Minor() != amount.Minor() {
			t.Errorf("balance update failed: got %v, want %v", w2.Balance, amount)
		}

		list, err := repo.ListTransactions(ctx, w.ID, 10, 0)
		if err != nil || len(list) != 1 {
			t.Errorf("expected 1 tx, got %d (err: %v)", len(list), err)
		}
	})

	t.Run("Payments_and_Methods", func(t *testing.T) {
		pm := &billing.UserPaymentMethod{
			UserID:            testBillingUserID,
			Provider:          "fawry",
			AccountIdentifier: "01000000000",
			IsDefault:         true,
		}
		if err := repo.AddPaymentMethod(ctx, pm); err != nil {
			t.Fatalf("AddPaymentMethod failed: %v", err)
		}

		pms, err := repo.ListPaymentMethods(ctx, testBillingUserID)
		if err != nil || len(pms) == 0 {
			t.Fatalf("ListPaymentMethods failed: %v", err)
		}

		orgID := testBillingOrgID
		pay := &billing.Payment{
			UserID:         testBillingUserID,
			OrganizationID: &orgID,
			Amount:         money.MustParse("250.00"),
			Method:         "fawry",
			Status:         "pending",
		}
		if err := repo.CreatePayment(ctx, pay); err != nil {
			t.Fatalf("CreatePayment failed: %v", err)
		}
		if pay.ID == 0 {
			t.Fatal("expected generated payment ID")
		}

		gotPay, err := repo.GetPaymentByID(ctx, pay.ID)
		if err != nil {
			t.Fatalf("GetPaymentByID failed: %v", err)
		}
		if gotPay.Amount.Minor() != pay.Amount.Minor() {
			t.Errorf("payment amount mismatch: got %v, want %v", gotPay.Amount, pay.Amount)
		}

		// A different user must not be able to delete this payment method by id.
		// billing.user_payment_methods has no row-level security, and the app
		// connects as a superuser in any case, so the user_id predicate in the
		// query is the only thing standing between one user and another's saved
		// card. Assert it, or a later refactor can quietly drop it.
		const otherUserID = testBillingUserID + 1
		if err := repo.DeletePaymentMethod(ctx, otherUserID, pm.ID); err == nil {
			t.Fatal("a different user deleted this payment method; the user_id predicate is missing")
		}
		if _, err := repo.ListPaymentMethods(ctx, testBillingUserID); err != nil {
			t.Fatalf("ListPaymentMethods failed: %v", err)
		}

		if err := repo.DeletePaymentMethod(ctx, testBillingUserID, pm.ID); err != nil {
			t.Fatalf("DeletePaymentMethod failed: %v", err)
		}
	})

	t.Run("Invoices_CRUD", func(t *testing.T) {
		ctxOrg := database.WithTenant(ctx, testBillingOrgID)
		subtotal := money.MustParse("500.00")
		inv := &billing.Invoice{
			OrganizationID: testBillingOrgID,
			InvoiceNumber:  "INV-TEST-88491",
			Subtotal:       subtotal,
			Status:         billing.InvoiceDraft,
			IssueDate:      time.Now(),
			DueDate:        time.Now().AddDate(0, 1, 0),
		}

		err := repo.CreateInvoice(ctxOrg, inv)
		if err != nil {
			t.Fatalf("CreateInvoice failed: %v", err)
		}
		if inv.ID == 0 {
			t.Fatal("expected generated invoice ID")
		}

		readInv, err := repo.GetInvoiceByID(ctxOrg, inv.ID)
		if err != nil {
			t.Fatalf("GetInvoiceByID failed: %v", err)
		}
		if readInv.Subtotal.Minor() != subtotal.Minor() {
			t.Errorf("money round-trip failed for invoice: %v", readInv.Subtotal)
		}

		err = repo.UpdateInvoiceStatus(ctxOrg, inv.ID, billing.InvoicePaid)
		if err != nil {
			t.Fatalf("UpdateInvoiceStatus failed: %v", err)
		}

		list, err := repo.ListInvoicesByOrg(ctxOrg, testBillingOrgID, 10, 0)
		if err != nil || len(list) == 0 {
			t.Fatalf("ListInvoicesByOrg failed: %v", err)
		}
	})

	t.Run("Plans_and_Subscriptions", func(t *testing.T) {
		plans, err := repo.ListPlans(ctx)
		if err != nil {
			t.Fatalf("ListPlans failed: %v", err)
		}
		_ = plans

		hasEnt, val, err := repo.CheckEntitlement(ctx, testBillingUserID, "ai_matching")
		if err != nil {
			t.Fatalf("CheckEntitlement failed: %v", err)
		}
		if hasEnt {
			t.Errorf("expected false for entitlement without subscription, got %v (%s)", hasEnt, val)
		}
	})

	t.Run("Payment_Methods_CRUD", func(t *testing.T) {
		pm := &billing.UserPaymentMethod{
			UserID:            testBillingUserID,
			Provider:          "paymob",
			AccountIdentifier: "**** 1234",
			IsDefault:         true,
		}
		if err := repo.AddPaymentMethod(ctx, pm); err != nil {
			t.Fatalf("AddPaymentMethod failed: %v", err)
		}
		if pm.ID == 0 {
			t.Fatal("expected generated payment method ID")
		}

		pms, err := repo.ListPaymentMethods(ctx, testBillingUserID)
		if err != nil {
			t.Fatalf("ListPaymentMethods failed: %v", err)
		}
		if len(pms) == 0 {
			t.Fatal("expected at least 1 payment method")
		}

		// Delete by wrong user should report not found
		if err := repo.DeletePaymentMethod(ctx, 999999, pm.ID); err == nil {
			t.Error("expected error deleting payment method owned by someone else")
		}

		// Delete by owner
		if err := repo.DeletePaymentMethod(ctx, testBillingUserID, pm.ID); err != nil {
			t.Fatalf("DeletePaymentMethod failed: %v", err)
		}
	})
}
