package billing

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AdminListSubscriptions provides cross-tenant subscription listing.
func (s *Service) AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*Subscription, error) {
	return s.repo.AdminListSubscriptions(ctx, limit, offset)
}

// AdminListSubscriptionsWithTotal provides cross-tenant subscription listing with total count.
func (s *Service) AdminListSubscriptionsWithTotal(ctx context.Context, limit, offset int) ([]*Subscription, int, error) {
	return s.repo.AdminListSubscriptionsWithTotal(ctx, limit, offset)
}

// AdminAdjustWallet adjusts a wallet's balance.
func (s *Service) AdminAdjustWallet(ctx context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error {
	return s.repo.AdminAdjustWallet(ctx, walletID, amount, reason, actorID)
}

// AdminListPayments provides cross-tenant payment listing.
func (s *Service) AdminListPayments(ctx context.Context, limit, offset int) ([]*Payment, error) {
	return s.repo.AdminListPayments(ctx, limit, offset)
}

// EnsureAllOrgWallets guarantees all registered organizations have wallet rows.
func (s *Service) EnsureAllOrgWallets(ctx context.Context) error {
	return s.repo.EnsureAllOrgWallets(ctx)
}

// AdminListDetailedWallets returns all wallets enriched with user and organization metadata.
func (s *Service) AdminListDetailedWallets(ctx context.Context, filter WalletFilter) ([]*AdminWalletView, int, error) {
	return s.repo.AdminListDetailedWallets(ctx, filter)
}

// AdminListDetailedTransactions returns wallet transactions with filters and metadata.
func (s *Service) AdminListDetailedTransactions(ctx context.Context, filter TransactionFilter) ([]*AdminWalletTransactionView, int, error) {
	return s.repo.AdminListDetailedTransactions(ctx, filter)
}

// AdminListDetailedInvoices returns invoices enriched with legal names and order details.
func (s *Service) AdminListDetailedInvoices(ctx context.Context, filter InvoiceFilter) ([]*AdminInvoiceView, int, error) {
	return s.repo.AdminListDetailedInvoices(ctx, filter)
}

// AdminListDetailedPayments returns payments enriched with order and tenant details.
func (s *Service) AdminListDetailedPayments(ctx context.Context, filter PaymentFilter) ([]*AdminPaymentView, int, error) {
	return s.repo.AdminListDetailedPayments(ctx, filter)
}

// AdminGetFinanceStats returns exact platform financial KPIs queried from the database.
func (s *Service) AdminGetFinanceStats(ctx context.Context) (*AdminFinanceStats, error) {
	return s.repo.AdminGetFinanceStats(ctx)
}

// AdminPerformWalletAdjustment executes a deposit, withdrawal, or balance adjustment.
func (s *Service) AdminPerformWalletAdjustment(ctx context.Context, walletID int64, amount money.Amount, txType TransactionType, reason string, actorID int64) error {
	return s.repo.AdminPerformWalletAdjustment(ctx, walletID, amount, txType, reason, actorID)
}

// AdminListDetailedDeposits returns deposit requests enriched with user, tenant, and reviewer details.
func (s *Service) AdminListDetailedDeposits(ctx context.Context, filter DepositFilter) ([]*AdminWalletDepositView, int, error) {
	return s.repo.AdminListDetailedDeposits(ctx, filter)
}

// AdminApproveDeposit approves a pending deposit request, crediting the user's wallet ledger.
func (s *Service) AdminApproveDeposit(ctx context.Context, depositID int64, reviewerID int64) (*WalletDeposit, *WalletTransaction, error) {
	dep, tx, err := s.repo.AdminApproveDepositRequest(ctx, depositID, reviewerID)
	if err != nil {
		return nil, nil, err
	}
	s.log.InfoContext(ctx, "admin approved wallet deposit", "deposit_id", depositID, "reviewer_id", reviewerID, "amount", dep.Amount.String())
	return dep, tx, nil
}

// AdminRejectDeposit rejects a pending deposit request with an explanatory reason.
func (s *Service) AdminRejectDeposit(ctx context.Context, depositID int64, reviewerID int64, reason string) (*WalletDeposit, error) {
	if reason == "" {
		reason = i18n.TDefault("w4_mod.s_250_250")
	}
	dep, err := s.repo.AdminRejectDepositRequest(ctx, depositID, reviewerID, reason)
	if err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "admin rejected wallet deposit", "deposit_id", depositID, "reviewer_id", reviewerID, "reason", reason)
	return dep, nil
}

// AdminListDetailedWithdrawals returns withdrawal requests enriched with user, tenant, and reviewer details.
func (s *Service) AdminListDetailedWithdrawals(ctx context.Context, filter WithdrawalFilter) ([]*AdminWalletWithdrawalView, int, error) {
	return s.repo.AdminListDetailedWithdrawals(ctx, filter)
}

// AdminApproveWithdrawal approves a pending withdrawal request, debiting the user's wallet ledger.
func (s *Service) AdminApproveWithdrawal(ctx context.Context, withdrawalID int64, reviewerID int64, transferReceiptURL string) (*WalletWithdrawal, *WalletTransaction, error) {
	w, tx, err := s.repo.AdminApproveWithdrawalRequest(ctx, withdrawalID, reviewerID, transferReceiptURL)
	if err != nil {
		return nil, nil, err
	}
	s.log.InfoContext(ctx, "admin approved wallet withdrawal", "withdrawal_id", withdrawalID, "reviewer_id", reviewerID, "amount", w.Amount.String(), "has_receipt", transferReceiptURL != "")
	return w, tx, nil
}

// AdminRejectWithdrawal rejects a pending withdrawal request with an explanatory reason.
func (s *Service) AdminRejectWithdrawal(ctx context.Context, withdrawalID int64, reviewerID int64, reason string) (*WalletWithdrawal, error) {
	if reason == "" {
		reason = "تم رفض طلب السحب من قبل الإدارة"
	}
	w, err := s.repo.AdminRejectWithdrawalRequest(ctx, withdrawalID, reviewerID, reason)
	if err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "admin rejected wallet withdrawal", "withdrawal_id", withdrawalID, "reviewerID", reviewerID, "reason", reason)
	return w, nil
}

// AdminRefundTransaction refunds an existing transaction by writing a compensating refund transaction row.
func (s *Service) AdminRefundTransaction(ctx context.Context, transactionID int64, reason string, actorID int64) (*WalletTransaction, error) {
	tx, err := s.repo.AdminRefundTransaction(ctx, transactionID, reason, actorID)
	if err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "admin refunded wallet transaction", "transaction_id", transactionID, "refund_tx_id", tx.ID, "amount", tx.Amount.String(), "actor_id", actorID)
	return tx, nil
}

// AdminRefundDeposit refunds an approved deposit request and its associated wallet transaction.
func (s *Service) AdminRefundDeposit(ctx context.Context, depositID int64, reason string, actorID int64) (*WalletDeposit, *WalletTransaction, error) {
	dep, tx, err := s.repo.AdminRefundDeposit(ctx, depositID, reason, actorID)
	if err != nil {
		return nil, nil, err
	}
	s.log.InfoContext(ctx, "admin refunded wallet deposit", "deposit_id", depositID, "refund_tx_id", tx.ID, "amount", tx.Amount.String(), "actor_id", actorID)
	return dep, tx, nil
}
