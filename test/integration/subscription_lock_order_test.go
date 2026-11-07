package integration_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// B1: a subscription renewal and a wallet subscription purchase touch the same
// wallet and subscription rows. Both must lock the wallet first, or Postgres can
// abort one of them as a deadlock. Run twenty rounds with both released at once.
func TestRenewalAndWalletPurchaseNeverDeadlock20Rounds(t *testing.T) {
	w := newWRWorld(t)
	ctx := database.AsSystem(context.Background())

	var planID int64
	if err := w.db.Pool().QueryRow(ctx, `SELECT id FROM billing.plans WHERE is_active ORDER BY id LIMIT 1;`).Scan(&planID); err != nil {
		t.Fatalf("no active plan: %v", err)
	}
	newSub := func(now time.Time) billing.Subscription {
		org := w.buyerOrg
		return billing.Subscription{
			UserID:         w.buyerUser,
			OrganizationID: &org,
			PlanID:         planID,
			Status:         billing.SubActive,
			BillingCycle:   "monthly",
			StartsAt:       now,
			ExpiresAt:      now.AddDate(0, 1, 0),
			SourceSystem:   "wallet_checkout",
		}
	}
	first, err := w.billRepo.PurchaseWalletSubscription(ctx, billing.WalletSubscriptionPurchase{
		WalletID:     w.walletID,
		Subscription: newSub(time.Now().UTC()),
		Delta:        money.Zero,
	})
	if err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	currentID := first.ID

	for round := 1; round <= 20; round++ {
		start := make(chan struct{})
		var wg sync.WaitGroup
		var renewErr, purchaseErr error
		var purchased *billing.Subscription
		expected := currentID

		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			renewErr = w.billRepo.RenewSubscription(ctx, expected, w.walletID, money.FromMinor(100), time.Now().UTC().AddDate(0, 1, 0), "b1 round")
		}()
		go func() {
			defer wg.Done()
			<-start
			purchased, purchaseErr = w.billRepo.PurchaseWalletSubscription(ctx, billing.WalletSubscriptionPurchase{
				WalletID:             w.walletID,
				Subscription:         newSub(time.Now().UTC()),
				ExpectedCurrentSubID: &expected,
				Delta:                money.Zero,
			})
		}()
		close(start)
		wg.Wait()

		for name, err := range map[string]error{"renewal": renewErr, "purchase": purchaseErr} {
			if err != nil && (strings.Contains(err.Error(), "40P01") || strings.Contains(strings.ToLower(err.Error()), "deadlock")) {
				t.Fatalf("round %d: %s hit a deadlock: %v", round, name, err)
			}
		}
		if renewErr != nil {
			t.Fatalf("round %d: renewal failed: %v", round, renewErr)
		}
		if purchaseErr == nil {
			currentID = purchased.ID
		}
	}

	// The renewal debits 1.00 each round, and nothing else moves money.
	var balance money.Amount
	if err := w.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT COALESCE(SUM(amount), 0.00) FROM billing.wallet_transactions WHERE wallet_id = $1;`, w.walletID).Scan(&balance)
	}); err != nil {
		t.Fatal(err)
	}
	if want := money.FromMinor(100000 - 20*100); balance.Minor() != want.Minor() {
		t.Fatalf("wallet balance %s after 20 renewals, want %s", balance, want)
	}
}
