package billing_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestWithdrawalWorkflow_HoldDeductRefundLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newMockBillingRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := billing.NewService(repo, logger)

	userID := int64(10)
	adminID := int64(99)

	// Step 0: Set initial wallet balance = 1,000.00 EGP
	initBal, _ := money.Parse("1000.00")
	repo.wallet.Balance = initBal
	repo.wallet.AvailableBalance = initBal
	repo.wallet.PendingWithdrawal = money.Zero

	// Verify initial state: Available = 1000, Pending = 0, Total = 1000
	if repo.wallet.AvailableBalance.Minor() != 100000 || repo.wallet.PendingWithdrawal.Minor() != 0 || repo.wallet.Balance.Minor() != 100000 {
		t.Fatalf("initial state incorrect: available=%s, pending=%s, total=%s",
			repo.wallet.AvailableBalance.String(), repo.wallet.PendingWithdrawal.String(), repo.wallet.Balance.String())
	}

	// Step 1: User requests withdrawal of 400.00 EGP
	w1Amt, _ := money.Parse("400.00")
	w1, err := svc.RequestWithdrawal(ctx, userID, nil, "EGP", w1Amt, "bank", "CIB EG123456", nil, "Withdrawal 1")
	if err != nil {
		t.Fatalf("RequestWithdrawal 1 failed: %v", err)
	}
	if w1.Status != billing.WithdrawalPending {
		t.Fatalf("expected pending status, got: %s", w1.Status)
	}

	// Now: Available = 600, Pending = 400, Total = 1000
	if repo.wallet.AvailableBalance.Minor() != 60000 {
		t.Fatalf("expected available balance 600.00 after withdrawal request, got: %s", repo.wallet.AvailableBalance.String())
	}
	if repo.wallet.PendingWithdrawal.Minor() != 40000 {
		t.Fatalf("expected pending withdrawal 400.00, got: %s", repo.wallet.PendingWithdrawal.String())
	}
	if repo.wallet.Balance.Minor() != 100000 {
		t.Fatalf("total ledger balance changed prematurely! Got: %s", repo.wallet.Balance.String())
	}

	// Step 2: Attempt withdrawal of 700.00 EGP (Exceeds available balance 600.00) -> Must fail
	wExcessAmt, _ := money.Parse("700.00")
	_, err = svc.RequestWithdrawal(ctx, userID, nil, "EGP", wExcessAmt, "bank", "CIB EG123456", nil, "Excess")
	if err == nil {
		t.Fatalf("expected error requesting withdrawal exceeding available balance, got nil")
	}

	// Step 3: User requests second withdrawal of 600.00 EGP (Exactly equals remaining available)
	w2Amt, _ := money.Parse("600.00")
	w2, err := svc.RequestWithdrawal(ctx, userID, nil, "EGP", w2Amt, "instapay", "user@instapay", nil, "Withdrawal 2")
	if err != nil {
		t.Fatalf("RequestWithdrawal 2 failed: %v", err)
	}
	if w2.Status != billing.WithdrawalPending {
		t.Fatalf("expected pending status, got: %s", w2.Status)
	}

	// Now: Available = 0, Pending = 1000, Total = 1000
	if repo.wallet.AvailableBalance.Minor() != 0 {
		t.Fatalf("expected available balance 0.00, got: %s", repo.wallet.AvailableBalance.String())
	}
	if repo.wallet.PendingWithdrawal.Minor() != 100000 {
		t.Fatalf("expected pending withdrawal 1000.00, got: %s", repo.wallet.PendingWithdrawal.String())
	}

	// Step 4: Admin REJECTS withdrawal 2 (600.00 EGP) -> Funds return to available balance
	rejReason := "Invalid InstaPay handle"
	rejW2, err := svc.AdminRejectWithdrawal(ctx, w2.ID, adminID, rejReason)
	if err != nil {
		t.Fatalf("AdminRejectWithdrawal failed: %v", err)
	}
	if rejW2.Status != billing.WithdrawalRejected {
		t.Fatalf("expected rejected status, got: %s", rejW2.Status)
	}

	// After Rejection: Available = 600 (returned!), Pending = 400, Total = 1000
	if repo.wallet.AvailableBalance.Minor() != 60000 {
		t.Fatalf("expected available balance returned to 600.00 after rejection, got: %s", repo.wallet.AvailableBalance.String())
	}
	if repo.wallet.PendingWithdrawal.Minor() != 40000 {
		t.Fatalf("expected pending withdrawal reduced to 400.00, got: %s", repo.wallet.PendingWithdrawal.String())
	}
	if repo.wallet.Balance.Minor() != 100000 {
		t.Fatalf("expected total balance 1000.00, got: %s", repo.wallet.Balance.String())
	}

	// Step 5: Admin APPROVES withdrawal 1 (400.00 EGP) -> Deducted from both total and pending!
	apprW1, tx, err := svc.AdminApproveWithdrawal(ctx, w1.ID, adminID, "/uploads/receipts/w1_proof.png")
	if err != nil {
		t.Fatalf("AdminApproveWithdrawal failed: %v", err)
	}
	if apprW1.Status != billing.WithdrawalApproved {
		t.Fatalf("expected approved status, got: %s", apprW1.Status)
	}
	if apprW1.TransferReceiptURL != "/uploads/receipts/w1_proof.png" {
		t.Fatalf("expected receipt URL /uploads/receipts/w1_proof.png, got: %s", apprW1.TransferReceiptURL)
	}
	if tx == nil || tx.Amount.Minor() != -40000 {
		t.Fatalf("expected negative debit transaction of -400.00, got: %v", tx)
	}

	// After Approval: Total = 600 (debited -400), Pending = 0 (cleared -400), Available = 600!
	if repo.wallet.Balance.Minor() != 60000 {
		t.Fatalf("expected total ledger balance 600.00 after approval, got: %s", repo.wallet.Balance.String())
	}
	if repo.wallet.PendingWithdrawal.Minor() != 0 {
		t.Fatalf("expected pending withdrawal 0.00 after approval, got: %s", repo.wallet.PendingWithdrawal.String())
	}
	if repo.wallet.AvailableBalance.Minor() != 60000 {
		t.Fatalf("expected available balance 600.00, got: %s", repo.wallet.AvailableBalance.String())
	}
}
