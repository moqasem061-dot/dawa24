package postgres_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/billing/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestAdminDepositRejectionPersistsWithoutCreditingWallet(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetBillingFixtures(t, db)
	ctx := database.AsSystem(context.Background())
	repo := postgres.NewRepository(db)
	wallet, err := repo.GetOrCreateWallet(ctx, testBillingUserID, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	deposit := &billing.WalletDeposit{WalletID: wallet.ID, UserID: testBillingUserID,
		OrganizationID: ptrBillingOrg(), Amount: money.MustParse("2111"), Currency: "EGP",
		PaymentMethod: "instapay", ReferenceNumber: "rejection-regression"}
	if err := repo.CreateDepositRequest(ctx, deposit); err != nil {
		t.Fatal(err)
	}
	reason := "لم يصل التحويل"
	rejected, err := repo.AdminRejectDepositRequest(ctx, deposit.ID, testBillingUserID, reason)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != billing.DepositRejected || rejected.RejectionReason != reason {
		t.Fatalf("%+v", rejected)
	}
	saved, err := repo.GetDepositRequestByID(ctx, deposit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != billing.DepositRejected || saved.ReviewedBy == nil || *saved.ReviewedBy != testBillingUserID || saved.ReviewedAt == nil || saved.RejectionReason != reason || saved.TransactionID != nil {
		t.Fatalf("rejection not persisted: %+v", saved)
	}
	if _, err := repo.AdminRejectDepositRequest(ctx, deposit.ID, testBillingUserID, "again"); err == nil {
		t.Fatal("rejected twice")
	}
	if _, _, err := repo.AdminApproveDepositRequest(ctx, deposit.ID, testBillingUserID); err == nil {
		t.Fatal("approved rejected deposit")
	}
	err = db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM billing.wallet_transactions WHERE wallet_id=$1`, wallet.ID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("rejection wrote %d wallet transactions", count)
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM platform.audit_log WHERE action='commerce.deposit.reject' AND entity_id=$1`, strconv.FormatInt(deposit.ID, 10)).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("audit count=%d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func ptrBillingOrg() *int64 { id := testBillingOrgID; return &id }
