package billing_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockBillingRepo struct {
	billing.Repository
	deposits       map[int64]*billing.WalletDeposit
	withdrawals    map[int64]*billing.WalletWithdrawal
	wallet         *billing.Wallet
	transactions   []*billing.WalletTransaction
	nextDepositID  int64
	nextWithdrawID int64
	nextTxID       int64
}

func newMockBillingRepo() *mockBillingRepo {
	zero := money.Zero
	return &mockBillingRepo{
		deposits:    make(map[int64]*billing.WalletDeposit),
		withdrawals: make(map[int64]*billing.WalletWithdrawal),
		wallet: &billing.Wallet{
			ID:                1,
			UserID:            10,
			Currency:          "EGP",
			Balance:           zero,
			PendingWithdrawal: zero,
			AvailableBalance:  zero,
		},
		nextDepositID:  1,
		nextWithdrawID: 1,
		nextTxID:       1,
	}
}

func (m *mockBillingRepo) GetOrCreateWallet(ctx context.Context, userID int64, currency string) (*billing.Wallet, error) {
	return m.wallet, nil
}

func (m *mockBillingRepo) CreateDepositRequest(ctx context.Context, dep *billing.WalletDeposit) error {
	dep.ID = m.nextDepositID
	m.nextDepositID++
	dep.Status = billing.DepositPending
	dep.CreatedAt = time.Now()
	dep.UpdatedAt = time.Now()
	m.deposits[dep.ID] = dep
	return nil
}

func (m *mockBillingRepo) GetDepositRequestByID(ctx context.Context, id int64) (*billing.WalletDeposit, error) {
	dep, ok := m.deposits[id]
	if !ok {
		return nil, apperr.NotFound("deposit")
	}
	return dep, nil
}

func (m *mockBillingRepo) UpdatePendingDepositRequest(ctx context.Context, dep *billing.WalletDeposit) error {
	existing, ok := m.deposits[dep.ID]
	if !ok || existing.Status != billing.DepositPending {
		return apperr.NotFound("deposit")
	}
	existing.Amount = dep.Amount
	existing.PaymentMethod = dep.PaymentMethod
	existing.ReferenceNumber = dep.ReferenceNumber
	existing.AttachmentURL = dep.AttachmentURL
	existing.UserNotes = dep.UserNotes
	existing.UpdatedAt = time.Now()
	return nil
}

func (m *mockBillingRepo) ListDepositRequestsByUser(ctx context.Context, userID int64, limit, offset int) ([]*billing.WalletDeposit, error) {
	var list []*billing.WalletDeposit
	for _, d := range m.deposits {
		if d.UserID == userID {
			list = append(list, d)
		}
	}
	return list, nil
}

func (m *mockBillingRepo) AdminApproveDepositRequest(ctx context.Context, depositID int64, reviewerID int64) (*billing.WalletDeposit, *billing.WalletTransaction, error) {
	dep, ok := m.deposits[depositID]
	if !ok || dep.Status != billing.DepositPending {
		return nil, nil, apperr.NotFound("deposit")
	}
	now := time.Now()
	dep.Status = billing.DepositApproved
	dep.ReviewedBy = &reviewerID
	dep.ReviewedAt = &now
	dep.TransactionID = &m.nextTxID

	newBal, _ := m.wallet.Balance.Add(dep.Amount)
	m.wallet.Balance = newBal

	tx := &billing.WalletTransaction{
		ID:           m.nextTxID,
		WalletID:     m.wallet.ID,
		Type:         billing.TxDeposit,
		Amount:       dep.Amount,
		BalanceAfter: newBal,
		CreatedAt:    now,
	}
	m.nextTxID++
	m.transactions = append(m.transactions, tx)
	return dep, tx, nil
}

func (m *mockBillingRepo) AdminRejectDepositRequest(ctx context.Context, depositID int64, reviewerID int64, reason string) (*billing.WalletDeposit, error) {
	dep, ok := m.deposits[depositID]
	if !ok || dep.Status != billing.DepositPending {
		return nil, apperr.NotFound("deposit")
	}
	now := time.Now()
	dep.Status = billing.DepositRejected
	dep.RejectionReason = reason
	dep.ReviewedBy = &reviewerID
	dep.ReviewedAt = &now
	return dep, nil
}

func (m *mockBillingRepo) CreateWithdrawalRequest(_ context.Context, w *billing.WalletWithdrawal) error {
	availMinor := m.wallet.Balance.Minor() - m.wallet.PendingWithdrawal.Minor()
	if availMinor < w.Amount.Minor() {
		return apperr.Validation("wallet.insufficient_funds", "رصيد المحفظة المتاح غير كافٍ لإتمام طلب السحب.", nil)
	}
	w.ID = m.nextWithdrawID
	m.nextWithdrawID++
	w.Status = billing.WithdrawalPending
	w.CreatedAt = time.Now()
	w.UpdatedAt = time.Now()
	if m.withdrawals == nil {
		m.withdrawals = make(map[int64]*billing.WalletWithdrawal)
	}
	m.withdrawals[w.ID] = w
	m.wallet.PendingWithdrawal, _ = m.wallet.PendingWithdrawal.Add(w.Amount)
	availMinor = m.wallet.Balance.Minor() - m.wallet.PendingWithdrawal.Minor()
	if availMinor < 0 {
		availMinor = 0
	}
	m.wallet.AvailableBalance = money.FromMinor(availMinor)
	return nil
}

func (m *mockBillingRepo) GetWithdrawalRequestByID(_ context.Context, id int64) (*billing.WalletWithdrawal, error) {
	w, ok := m.withdrawals[id]
	if !ok {
		return nil, apperr.NotFound("withdrawal")
	}
	return w, nil
}

func (m *mockBillingRepo) ListWithdrawalRequestsByUserWithStatus(_ context.Context, _ int64, _ string, _, _ int) ([]*billing.WalletWithdrawal, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminListDetailedWithdrawals(_ context.Context, _ billing.WithdrawalFilter) ([]*billing.AdminWalletWithdrawalView, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) AdminApproveWithdrawalRequest(_ context.Context, id int64, reviewerID int64, transferReceiptURL string) (*billing.WalletWithdrawal, *billing.WalletTransaction, error) {
	w, ok := m.withdrawals[id]
	if !ok {
		return nil, nil, apperr.NotFound("withdrawal")
	}
	if w.Status != billing.WithdrawalPending {
		return nil, nil, apperr.Conflict("withdrawal.already_processed", "already processed")
	}
	w.Status = billing.WithdrawalApproved
	w.TransferReceiptURL = transferReceiptURL
	now := time.Now()
	w.ReviewedBy = &reviewerID
	w.ReviewedAt = &now

	negDelta, _ := money.Zero.Sub(w.Amount)
	newBal, _ := m.wallet.Balance.Add(negDelta)
	m.wallet.Balance = newBal
	penMinor := m.wallet.PendingWithdrawal.Minor() - w.Amount.Minor()
	if penMinor < 0 {
		penMinor = 0
	}
	m.wallet.PendingWithdrawal = money.FromMinor(penMinor)
	availMinor := m.wallet.Balance.Minor() - m.wallet.PendingWithdrawal.Minor()
	if availMinor < 0 {
		availMinor = 0
	}
	m.wallet.AvailableBalance = money.FromMinor(availMinor)

	tx := &billing.WalletTransaction{
		ID:           m.nextTxID,
		WalletID:     m.wallet.ID,
		Type:         billing.TxWithdrawal,
		Amount:       negDelta,
		BalanceAfter: newBal,
		CreatedAt:    now,
	}
	m.nextTxID++
	w.TransactionID = &tx.ID
	return w, tx, nil
}

func (m *mockBillingRepo) AdminRejectWithdrawalRequest(_ context.Context, id int64, reviewerID int64, reason string) (*billing.WalletWithdrawal, error) {
	w, ok := m.withdrawals[id]
	if !ok {
		return nil, apperr.NotFound("withdrawal")
	}
	if w.Status != billing.WithdrawalPending {
		return nil, apperr.Conflict("withdrawal.already_processed", "already processed")
	}
	w.Status = billing.WithdrawalRejected
	now := time.Now()
	w.ReviewedBy = &reviewerID
	w.ReviewedAt = &now
	w.RejectionReason = reason

	penMinor := m.wallet.PendingWithdrawal.Minor() - w.Amount.Minor()
	if penMinor < 0 {
		penMinor = 0
	}
	m.wallet.PendingWithdrawal = money.FromMinor(penMinor)
	availMinor := m.wallet.Balance.Minor() - m.wallet.PendingWithdrawal.Minor()
	if availMinor < 0 {
		availMinor = 0
	}
	m.wallet.AvailableBalance = money.FromMinor(availMinor)
	return w, nil
}

func TestDepositWorkflowLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newMockBillingRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := billing.NewService(repo, logger)

	amt, _ := money.Parse("1000.00")
	userID := int64(10)
	adminID := int64(99)

	// 1. User Requests Deposit
	dep, err := svc.RequestDeposit(ctx, userID, nil, "EGP", amt, "instapay", "REF-100", "/receipt.png", "Deposit test")
	if err != nil {
		t.Fatalf("RequestDeposit failed: %v", err)
	}
	if dep.Status != billing.DepositPending {
		t.Fatalf("expected pending status, got %s", dep.Status)
	}
	if repo.wallet.Balance.Minor() != 0 {
		t.Fatalf("wallet credited before approval!")
	}

	// 2. User edits pending deposit
	newAmt, _ := money.Parse("1500.00")
	updated, err := svc.EditPendingDeposit(ctx, userID, dep.ID, newAmt, "instapay", "REF-100-EDIT", "/receipt2.png", "Updated note")
	if err != nil {
		t.Fatalf("EditPendingDeposit failed: %v", err)
	}
	if updated.Amount != newAmt {
		t.Fatalf("expected amount %s, got %s", newAmt.String(), updated.Amount.String())
	}

	// 3. Admin Approves Deposit
	approved, tx, err := svc.AdminApproveDeposit(ctx, dep.ID, adminID)
	if err != nil {
		t.Fatalf("AdminApproveDeposit failed: %v", err)
	}
	if approved.Status != billing.DepositApproved {
		t.Fatalf("expected approved status, got %s", approved.Status)
	}
	if repo.wallet.Balance != newAmt {
		t.Fatalf("expected wallet balance %s, got %s", newAmt.String(), repo.wallet.Balance.String())
	}
	if tx.BalanceAfter != newAmt {
		t.Fatalf("expected tx balance_after %s, got %s", newAmt.String(), tx.BalanceAfter.String())
	}

	// 4. User CANNOT edit approved deposit
	_, err = svc.EditPendingDeposit(ctx, userID, dep.ID, newAmt, "instapay", "REF-TRY", "", "")
	if err == nil {
		t.Fatalf("expected error editing approved deposit, got nil")
	}

	// 5. User requests second deposit and admin rejects it
	dep2Amt, _ := money.Parse("500.00")
	dep2, err := svc.RequestDeposit(ctx, userID, nil, "EGP", dep2Amt, "bank_transfer", "REF-200", "", "Second deposit")
	if err != nil {
		t.Fatalf("RequestDeposit 2 failed: %v", err)
	}

	rejReason := "Invalid bank receipt"
	rejected, err := svc.AdminRejectDeposit(ctx, dep2.ID, adminID, rejReason)
	if err != nil {
		t.Fatalf("AdminRejectDeposit failed: %v", err)
	}
	if rejected.Status != billing.DepositRejected {
		t.Fatalf("expected rejected status, got %s", rejected.Status)
	}
	if rejected.RejectionReason != rejReason {
		t.Fatalf("expected reason %s, got %s", rejReason, rejected.RejectionReason)
	}

	// Wallet balance remains unchanged after rejection
	if repo.wallet.Balance != newAmt {
		t.Fatalf("wallet balance changed on rejection! Expected %s, got %s", newAmt.String(), repo.wallet.Balance.String())
	}

	// User CANNOT edit rejected deposit
	_, err = svc.EditPendingDeposit(ctx, userID, dep2.ID, dep2Amt, "bank_transfer", "REF-TRY-2", "", "")
	if err == nil {
		t.Fatalf("expected error editing rejected deposit, got nil")
	}
}

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
