package billing_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
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
