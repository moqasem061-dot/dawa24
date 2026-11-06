package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// CreditOrderRefund credits back, inside the caller's transaction, the wallet
// money still net-debited for an order. The caller (the commerce repository)
// has already locked the order row, so the decision is taken from the locked
// order and the ledger, never from a stale copy.
//
// The amount is what the ledger shows as debited for the order, less earlier
// order credits and less admin reversals of the order's debits. It is never the
// order total. capAmount, when given, lowers it (a supplier's partial cancel).
// A non-empty key is stored on the credit row; the partial unique index allows
// one row per key, so one cancellation event credits at most once. A key that
// is already recorded is refused with a conflict and nothing is credited.
//
// The wallet row is locked before the balance is read, so concurrent refunds
// for one wallet serialise here and in the admin finance refund.
func (r *Repository) CreditOrderRefund(
	ctx context.Context,
	tx pgx.Tx,
	orderID int64,
	capAmount *money.Amount,
	key, description string,
	actorID int64,
) (money.Amount, error) {
	walletIDs, err := orderDebitWallets(ctx, tx, orderID)
	if err != nil {
		return money.Zero, err
	}
	switch len(walletIDs) {
	case 0:
		return money.Zero, nil
	case 1:
	default:
		return money.Zero, apperr.Internal(fmt.Errorf("billing: order %d was debited from %d wallets; refund needs review", orderID, len(walletIDs)))
	}
	walletID := walletIDs[0]
	if err := lockWalletRow(ctx, tx, walletID); err != nil {
		return money.Zero, err
	}
	remaining, err := orderRemaining(ctx, tx, orderID)
	if err != nil {
		return money.Zero, err
	}
	amount := remaining
	if capAmount != nil && capAmount.Minor() < amount.Minor() {
		amount = *capAmount
	}
	if !amount.IsPositive() {
		return money.Zero, nil
	}
	if key != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.wallet_transactions WHERE idempotency_key = $1);`, key).Scan(&exists); err != nil {
			return money.Zero, err
		}
		if exists {
			return money.Zero, apperr.Conflict("order.refund_already_recorded", "This cancellation has already been refunded.")
		}
	}
	var latest money.Amount
	err = tx.QueryRow(ctx, `SELECT balance_after FROM billing.wallet_transactions WHERE wallet_id = $1 ORDER BY id DESC LIMIT 1;`, walletID).Scan(&latest)
	if err != nil && !database.IsNotFound(err) {
		return money.Zero, err
	}
	next, err := latest.Add(amount)
	if err != nil {
		return money.Zero, apperr.Internal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.wallet_transactions (
			wallet_id, type, amount, balance_after, reference_type, reference_id, description, idempotency_key
		) VALUES ($1, 'deposit', $2, $3, 'refund', $4, $5, NULLIF($6, ''));`,
		walletID, amount, next, orderID, description, key,
	); err != nil {
		return money.Zero, err
	}
	if err := database.WriteAudit(ctx, tx, database.AuditEntry{
		ActorUserID: actorID,
		Action:      "billing.wallet.order_refunded",
		EntityType:  "order",
		EntityID:    strconv.FormatInt(orderID, 10),
		Before:      map[string]string{"balance": latest.String(), "order_net_debit": remaining.String()},
		After:       map[string]string{"balance": next.String(), "refund_amount": amount.String(), "idempotency_key": key},
	}); err != nil {
		return money.Zero, err
	}
	return amount, nil
}

// orderRemaining returns the money still net-debited for an order, as the ledger
// shows it. Callers must hold the order's wallet lock (see lockedOrderRemaining).
func orderRemaining(ctx context.Context, tx pgx.Tx, orderID int64) (money.Amount, error) {
	var debited, reversed, refunded money.Amount
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(-amount), 0.00) FROM billing.wallet_transactions
		WHERE type = 'withdrawal' AND reference_type = 'order' AND reference_id = $1;`, orderID).Scan(&debited); err != nil {
		return money.Zero, err
	}
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(r.amount), 0.00) FROM billing.wallet_transactions r
		WHERE r.reverses_transaction_id IN (
			SELECT d.id FROM billing.wallet_transactions d
			WHERE d.type = 'withdrawal' AND d.reference_type = 'order' AND d.reference_id = $1
		);`, orderID).Scan(&reversed); err != nil {
		return money.Zero, err
	}
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0.00) FROM billing.wallet_transactions
		WHERE type = 'deposit' AND reference_type = 'refund' AND reference_id = $1;`, orderID).Scan(&refunded); err != nil {
		return money.Zero, err
	}
	remaining, err := debited.Sub(reversed)
	if err != nil {
		return money.Zero, apperr.Internal(err)
	}
	remaining, err = remaining.Sub(refunded)
	if err != nil {
		return money.Zero, apperr.Internal(err)
	}
	if remaining.IsNegative() {
		return money.Zero, nil
	}
	return remaining, nil
}

func orderDebitWallets(ctx context.Context, tx pgx.Tx, orderID int64) ([]int64, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT wallet_id FROM billing.wallet_transactions
		WHERE type = 'withdrawal' AND reference_type = 'order' AND reference_id = $1
		ORDER BY wallet_id;`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func lockWalletRow(ctx context.Context, tx pgx.Tx, walletID int64) error {
	var id int64
	return tx.QueryRow(ctx, `SELECT id FROM billing.wallets WHERE id = $1 FOR UPDATE;`, walletID).Scan(&id)
}

// lockedOrderRemaining locks the wallet of an order debit, then returns what the
// order still has net-debited. The admin finance refund uses it before crediting.
func lockedOrderRemaining(ctx context.Context, tx pgx.Tx, walletID, orderID int64) (money.Amount, error) {
	if err := lockWalletRow(ctx, tx, walletID); err != nil {
		return money.Zero, err
	}
	return orderRemaining(ctx, tx, orderID)
}
