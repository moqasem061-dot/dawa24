package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// recordLedgerRow appends one row to a wallet's ledger inside tx.
//
// The wallet row is locked before the balance is read. Under READ COMMITTED a
// balance read that is not preceded by that lock can see a stale latest row, so
// two debits could both start from the same balance and one of them would be
// lost (Batch 2, S01). With the lock, the second writer waits for the first to
// commit and then reads its row. A negative balance, or a debit that would dip
// into pending withdrawals, is refused and nothing is written.
//
// A non-empty key is stored on the row. The partial unique index on the key
// refuses a second row with the same key.
func (r *Repository) recordLedgerRow(
	ctx context.Context,
	tx pgx.Tx,
	walletID int64,
	txType billing.TransactionType,
	delta money.Amount,
	refType string,
	refID *int64,
	desc string,
	key string,
) (*billing.WalletTransaction, error) {
	if err := lockWalletRow(ctx, tx, walletID); err != nil {
		return nil, err
	}
	// The wallet lock already serialises writers, so the balance is a plain read. A
	// row lock on the latest ledger row here would order locks differently from the
	// admin refund, which locks a ledger row before the wallet, and could deadlock.
	currentBalance, err := r.latestBalance(ctx, tx, walletID, false)
	if err != nil {
		return nil, err
	}

	newBalance, addErr := currentBalance.Add(delta)
	if addErr != nil {
		return nil, apperr.Internal(addErr)
	}
	if newBalance.IsNegative() {
		return nil, apperr.Validation("wallet.insufficient_funds", "رصيد المحفظة غير كافٍ لإتمام هذه العملية.", nil)
	}

	// When debiting, ensure non-withdrawal transactions do not spend held pending withdrawal funds.
	if delta.IsNegative() && refType != "withdrawal_approval" {
		pendingWithdrawals, err := r.pendingWithdrawals(ctx, tx, walletID)
		if err != nil {
			return nil, err
		}
		availableMinor := currentBalance.Minor() - pendingWithdrawals.Minor()
		if availableMinor < (-delta.Minor()) {
			return nil, apperr.Validation("wallet.insufficient_funds", "رصيد المحفظة المتاح غير كافٍ لإتمام هذه العملية لوجود مبالغ معلقة.", nil)
		}
	}

	var rec billing.WalletTransaction
	var typeStr string
	err = tx.QueryRow(ctx, `
		INSERT INTO billing.wallet_transactions (
			wallet_id, type, amount, balance_after, reference_type, reference_id, description, idempotency_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''))
		RETURNING id, wallet_id, type, amount, balance_after, reference_type, reference_id, description, created_at;
	`,
		walletID, string(txType), delta, newBalance, refType, refID, desc, key,
	).Scan(
		&rec.ID, &rec.WalletID, &typeStr, &rec.Amount, &rec.BalanceAfter,
		&rec.ReferenceType, &rec.ReferenceID, &rec.Description, &rec.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	rec.Type = billing.TransactionType(typeStr)
	return &rec, nil
}

// WithdrawInTx debits a user's wallet inside the caller's transaction. The wallet
// lock and the balance and pending-withdrawal checks apply, so a refused debit
// returns an error and the caller's other writes roll back with it. It returns the
// id of the ledger row.
func (r *Repository) WithdrawInTx(
	ctx context.Context,
	tx pgx.Tx,
	userID int64,
	amount money.Amount,
	refType string,
	refID *int64,
	desc string,
) (int64, error) {
	if userID <= 0 {
		return 0, apperr.Validation("wallet.payer_required", "تعذر تحديد المحفظة المراد الخصم منها. The wallet to debit could not be identified.", nil)
	}
	if !amount.IsPositive() {
		return 0, apperr.Internal(fmt.Errorf("billing: withdrawal must be positive, got %s", amount.String()))
	}
	walletID, err := ensureWalletTx(ctx, tx, userID, "EGP")
	if err != nil {
		return 0, err
	}
	negDelta, err := money.Zero.Sub(amount)
	if err != nil {
		return 0, apperr.Internal(err)
	}
	rec, err := r.recordLedgerRow(ctx, tx, walletID, billing.TxWithdrawal, negDelta, refType, refID, desc, "")
	if err != nil {
		return 0, err
	}
	return rec.ID, nil
}
