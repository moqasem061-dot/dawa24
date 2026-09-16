package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type queryable interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (r *Repository) latestBalance(ctx context.Context, q queryable, walletID int64, forUpdate bool) (money.Amount, error) {
	var bal money.Amount
	var query string
	if forUpdate {
		query = `SELECT balance_after FROM billing.wallet_transactions WHERE wallet_id = $1 ORDER BY id DESC LIMIT 1 FOR UPDATE;`
	} else {
		query = `SELECT balance_after FROM billing.wallet_transactions WHERE wallet_id = $1 ORDER BY id DESC LIMIT 1;`
	}
	err := q.QueryRow(ctx, query, walletID).Scan(&bal)
	if err != nil {
		if database.IsNotFound(err) {
			return money.Zero, nil
		}
		return money.Amount{}, err
	}
	return bal, nil
}

func (r *Repository) pendingWithdrawals(ctx context.Context, q queryable, walletID int64) (money.Amount, error) {
	var pending money.Amount
	query := `SELECT COALESCE(SUM(amount), 0) FROM billing.wallet_withdrawals WHERE wallet_id = $1 AND status = 'pending';`
	err := q.QueryRow(ctx, query, walletID).Scan(&pending)
	if err != nil {
		if database.IsNotFound(err) {
			return money.Zero, nil
		}
		return money.Amount{}, err
	}
	return pending, nil
}
