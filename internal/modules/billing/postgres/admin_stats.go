package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AdminGetFinanceStats executes audited SQL queries backing each financial figure across the platform.
func (r *Repository) AdminGetFinanceStats(ctx context.Context) (*billing.AdminFinanceStats, error) {
	stats := &billing.AdminFinanceStats{}

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Payments: gross amount paid and count of successful payments
		const payQ = `
			SELECT COUNT(*), COALESCE(SUM(amount), 0)
			FROM billing.payments
			WHERE status IN ('paid', 'completed', 'success');
		`
		if err := tx.QueryRow(txCtx, payQ).Scan(&stats.TotalPayments, &stats.TotalPaid); err != nil {
			return err
		}

		// 2. Invoices: total invoiced amount and count of non-cancelled invoices
		const invQ = `
			SELECT COUNT(*), COALESCE(SUM(total_amount), 0)
			FROM billing.invoices
			WHERE status != 'cancelled';
		`
		if err := tx.QueryRow(txCtx, invQ).Scan(&stats.TotalInvoices, &stats.TotalRevenue); err != nil {
			return err
		}

		// 3. Wallets: total wallet count and sum of current balances from ledger
		const walCountQ = `SELECT COUNT(*) FROM billing.wallets;`
		if err := tx.QueryRow(txCtx, walCountQ).Scan(&stats.TotalWallets); err != nil {
			return err
		}

		const walHeldQ = `
			SELECT COALESCE(SUM(latest_balance), 0)
			FROM (
				SELECT DISTINCT ON (wallet_id) balance_after AS latest_balance
				FROM billing.wallet_transactions
				ORDER BY wallet_id, id DESC
			) t;
		`
		var held money.Amount
		if err := tx.QueryRow(txCtx, walHeldQ).Scan(&held); err != nil {
			return err
		}
		stats.TotalHeld = held

		// 4. Transactions: total count of recorded entries
		const txCountQ = `SELECT COUNT(*) FROM billing.wallet_transactions;`
		if err := tx.QueryRow(txCtx, txCountQ).Scan(&stats.TotalTransactions); err != nil {
			return err
		}

		// 5. Deposits: total and pending
		const depQ = `
			SELECT COUNT(*), COALESCE(COUNT(*) FILTER (WHERE status = 'pending'), 0)
			FROM billing.wallet_deposits;
		`
		if err := tx.QueryRow(txCtx, depQ).Scan(&stats.TotalDeposits, &stats.PendingDeposits); err != nil {
			return err
		}

		// 6. Withdrawals: total and pending
		const withQ = `
			SELECT COUNT(*), COALESCE(COUNT(*) FILTER (WHERE status = 'pending'), 0)
			FROM billing.wallet_withdrawals;
		`
		if err := tx.QueryRow(txCtx, withQ).Scan(&stats.TotalWithdrawals, &stats.PendingWithdrawals); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return stats, nil
}
