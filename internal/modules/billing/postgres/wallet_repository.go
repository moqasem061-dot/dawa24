package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Repository implements billing.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a new billing PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// GetOrCreateWallet retrieves or initializes a user wallet.
func (r *Repository) GetOrCreateWallet(ctx context.Context, userID int64, currency string) (*billing.Wallet, error) {
	if currency == "" {
		currency = "EGP"
	}
	var w billing.Wallet
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO billing.wallets (user_id, currency)
			VALUES ($1, $2)
			ON CONFLICT (user_id, currency) DO UPDATE SET updated_at = now()
			RETURNING id, public_id, user_id, organization_id, currency, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query, userID, currency).Scan(
			&w.ID, &w.PublicID, &w.UserID, &w.OrganizationID, &w.Currency, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return err
		}

		// Compute balance from latest transaction
		bal, err := r.latestBalance(txCtx, tx, w.ID, false)
		if err != nil {
			return err
		}
		w.Balance = bal

		// Compute pending withdrawals
		pend, err := r.pendingWithdrawals(txCtx, tx, w.ID)
		if err != nil {
			return err
		}
		w.PendingWithdrawal = pend

		availMinor := w.Balance.Minor() - w.PendingWithdrawal.Minor()
		if availMinor < 0 {
			availMinor = 0
		}
		w.AvailableBalance = money.FromMinor(availMinor)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("billing postgres: get or create wallet: %w", err)
	}
	return &w, nil
}

// GetWallet retrieves a wallet and computes its current balance, pending withdrawals, and available balance.
func (r *Repository) GetWallet(ctx context.Context, id int64) (*billing.Wallet, error) {
	var w billing.Wallet
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, user_id, organization_id, currency, created_at, updated_at FROM billing.wallets WHERE id = $1;`
		if err := tx.QueryRow(txCtx, query, id).Scan(
			&w.ID, &w.PublicID, &w.UserID, &w.OrganizationID, &w.Currency, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("wallet")
			}
			return err
		}

		bal, err := r.latestBalance(txCtx, tx, w.ID, false)
		if err != nil {
			return err
		}
		w.Balance = bal

		pend, err := r.pendingWithdrawals(txCtx, tx, w.ID)
		if err != nil {
			return err
		}
		w.PendingWithdrawal = pend

		availMinor := w.Balance.Minor() - w.PendingWithdrawal.Minor()
		if availMinor < 0 {
			availMinor = 0
		}
		w.AvailableBalance = money.FromMinor(availMinor)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// RecordTransaction writes an append-only ledger row and updates the balance projection.
func (r *Repository) RecordTransaction(
	ctx context.Context,
	walletID int64,
	txType billing.TransactionType,
	delta money.Amount,
	refType string,
	refID *int64,
	desc string,
) (*billing.WalletTransaction, error) {
	var txRecord billing.WalletTransaction
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		currentBalance, err := r.latestBalance(txCtx, tx, walletID, true)
		if err != nil {
			return err
		}

		newBalance, addErr := currentBalance.Add(delta)
		if addErr != nil {
			return apperr.Internal(addErr)
		}
		if newBalance.IsNegative() {
			return apperr.Validation("wallet.insufficient_funds", "رصيد المحفظة غير كافٍ لإتمام هذه العملية.", nil)
		}

		// When debiting, ensure non-withdrawal transactions do not spend held pending withdrawal funds
		if delta.IsNegative() && refType != "withdrawal_approval" {
			pendingWithdrawals, err := r.pendingWithdrawals(txCtx, tx, walletID)
			if err != nil {
				return err
			}

			availableMinor := currentBalance.Minor() - pendingWithdrawals.Minor()
			if availableMinor < (-delta.Minor()) {
				return apperr.Validation("wallet.insufficient_funds", "رصيد المحفظة المتاح غير كافٍ لإتمام هذه العملية لوجود مبالغ معلقة.", nil)
			}
		}

		queryInsert := `
			INSERT INTO billing.wallet_transactions (
				wallet_id, type, amount, balance_after, reference_type, reference_id, description
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, wallet_id, type, amount, balance_after, reference_type, reference_id, description, created_at;
		`
		var typeStr string
		err = tx.QueryRow(txCtx, queryInsert,
			walletID, string(txType), delta, newBalance, refType, refID, desc,
		).Scan(
			&txRecord.ID, &txRecord.WalletID, &typeStr, &txRecord.Amount, &txRecord.BalanceAfter,
			&txRecord.ReferenceType, &txRecord.ReferenceID, &txRecord.Description, &txRecord.CreatedAt,
		)
		if err != nil {
			return err
		}
		txRecord.Type = billing.TransactionType(typeStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &txRecord, nil
}

// ListTransactions retrieves paginated transactions for a wallet.
func (r *Repository) ListTransactions(ctx context.Context, walletID int64, limit, offset int) ([]*billing.WalletTransaction, error) {
	list, _, err := r.ListTransactionsWithTotal(ctx, walletID, limit, offset)
	return list, err
}

// ListTransactionsWithTotal retrieves paginated transactions for a wallet with total count.
func (r *Repository) ListTransactionsWithTotal(ctx context.Context, walletID int64, limit, offset int) ([]*billing.WalletTransaction, int, error) {
	var list []*billing.WalletTransaction
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `SELECT count(*) FROM billing.wallet_transactions WHERE wallet_id = $1;`
		if err := tx.QueryRow(txCtx, countQuery, walletID).Scan(&total); err != nil {
			return err
		}

		query := `
			SELECT id, wallet_id, type, amount, balance_after, reference_type, reference_id, description, created_at
			FROM billing.wallet_transactions
			WHERE wallet_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}
		rows, err := tx.Query(txCtx, query, walletID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var t billing.WalletTransaction
			var typeStr string
			if err := rows.Scan(
				&t.ID, &t.WalletID, &typeStr, &t.Amount, &t.BalanceAfter,
				&t.ReferenceType, &t.ReferenceID, &t.Description, &t.CreatedAt,
			); err != nil {
				return err
			}
			t.Type = billing.TransactionType(typeStr)
			list = append(list, &t)
		}
		return rows.Err()
	})
	return list, total, err
}

// ListTransactionsWithTypeTotal retrieves paginated wallet ledger rows,
// optionally restricted to one billing.wallet_transactions.type value.
// An empty or unknown txType behaves like the unfiltered listing.
func (r *Repository) ListTransactionsWithTypeTotal(ctx context.Context, walletID int64, txType string, limit, offset int) ([]*billing.WalletTransaction, int, error) {
	switch billing.TransactionType(txType) {
	case "", billing.TxDeposit, billing.TxWithdrawal, billing.TxPurchase,
		billing.TxRefund, billing.TxBonus, billing.TxPenalty,
		billing.TxTransferIn, billing.TxTransferOut, billing.TxAdjustment:
	default:
		txType = ""
	}
	var list []*billing.WalletTransaction
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		typePred := ""
		args := []any{walletID}
		if txType != "" {
			typePred = ` AND type = $2`
			args = append(args, txType)
		}
		countQuery := `SELECT count(*) FROM billing.wallet_transactions WHERE wallet_id = $1` + typePred + `;`
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}
		query := `
			SELECT id, wallet_id, type, amount, balance_after, reference_type, reference_id, description, created_at
			FROM billing.wallet_transactions
			WHERE wallet_id = $1` + typePred + `
			ORDER BY created_at DESC, id DESC
			LIMIT $` + fmt.Sprintf("%d", len(args)+1) + ` OFFSET $` + fmt.Sprintf("%d", len(args)+2) + `;
		`
		args = append(args, limit, offset)
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var t billing.WalletTransaction
			var typeStr string
			if err := rows.Scan(
				&t.ID, &t.WalletID, &typeStr, &t.Amount, &t.BalanceAfter,
				&t.ReferenceType, &t.ReferenceID, &t.Description, &t.CreatedAt,
			); err != nil {
				return err
			}
			t.Type = billing.TransactionType(typeStr)
			list = append(list, &t)
		}
		return rows.Err()
	})
	return list, total, err
}
