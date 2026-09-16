package postgres

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CreateWithdrawalRequest inserts a new withdrawal request in pending status after validating funds.
func (r *Repository) CreateWithdrawalRequest(ctx context.Context, w *billing.WalletWithdrawal) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Check current available balance
		currentBalance, err := r.latestBalance(txCtx, tx, w.WalletID, true)
		if err != nil {
			return fmt.Errorf("read wallet balance: %w", err)
		}

		// Calculate existing pending withdrawals
		pendingWithdrawals, err := r.pendingWithdrawals(txCtx, tx, w.WalletID)
		if err != nil {
			return fmt.Errorf("read pending withdrawals: %w", err)
		}

		availableMinor := currentBalance.Minor() - pendingWithdrawals.Minor()
		if availableMinor < w.Amount.Minor() {
			return apperr.Validation("wallet.insufficient_funds", "رصيد المحفظة المتاح غير كافٍ لإتمام طلب السحب.", nil)
		}

		query := `
			INSERT INTO billing.wallet_withdrawals (
				wallet_id, user_id, organization_id, amount, currency, payout_method_type,
				destination_details, user_payment_method_id, user_notes, status
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending')
			RETURNING id, public_id, status, created_at, updated_at;
		`
		var statusStr string
		if err := tx.QueryRow(
			txCtx, query,
			w.WalletID, w.UserID, w.OrganizationID, w.Amount, w.Currency,
			w.PayoutMethodType, w.DestinationDetails, w.UserPaymentMethodID, w.UserNotes,
		).Scan(&w.ID, &w.PublicID, &statusStr, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return fmt.Errorf("create withdrawal request: %w", err)
		}
		w.Status = billing.WithdrawalStatus(statusStr)
		return nil
	})
}

// GetWithdrawalRequestByID retrieves a withdrawal request by ID.
func (r *Repository) GetWithdrawalRequestByID(ctx context.Context, id int64) (*billing.WalletWithdrawal, error) {
	var w billing.WalletWithdrawal
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payout_method_type, destination_details, user_payment_method_id, COALESCE(user_notes, ''),
			       status, COALESCE(rejection_reason, ''), reviewed_by, reviewed_at, transaction_id,
			       COALESCE(transfer_receipt_url, ''), created_at, updated_at
			FROM billing.wallet_withdrawals
			WHERE id = $1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&w.ID, &w.PublicID, &w.WalletID, &w.UserID, &w.OrganizationID, &w.Amount, &w.Currency,
			&w.PayoutMethodType, &w.DestinationDetails, &w.UserPaymentMethodID, &w.UserNotes,
			&statusStr, &w.RejectionReason, &w.ReviewedBy, &w.ReviewedAt, &w.TransactionID,
			&w.TransferReceiptURL, &w.CreatedAt, &w.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("withdrawal_request")
			}
			return err
		}
		w.Status = billing.WithdrawalStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// ListWithdrawalRequestsByUserWithStatus returns user withdrawal requests filtered by status.
func (r *Repository) ListWithdrawalRequestsByUserWithStatus(
	ctx context.Context, userID int64, status string, limit, offset int,
) ([]*billing.WalletWithdrawal, error) {
	var list []*billing.WalletWithdrawal
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payout_method_type, destination_details, user_payment_method_id, COALESCE(user_notes, ''),
			       status, COALESCE(rejection_reason, ''), reviewed_by, reviewed_at, transaction_id,
			       COALESCE(transfer_receipt_url, ''), created_at, updated_at
			FROM billing.wallet_withdrawals
			WHERE user_id = $1
			  AND ($2 = '' OR status = $2)
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4;
		`
		rows, err := tx.Query(txCtx, query, userID, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var w billing.WalletWithdrawal
			var statusStr string
			if err := rows.Scan(
				&w.ID, &w.PublicID, &w.WalletID, &w.UserID, &w.OrganizationID, &w.Amount, &w.Currency,
				&w.PayoutMethodType, &w.DestinationDetails, &w.UserPaymentMethodID, &w.UserNotes,
				&statusStr, &w.RejectionReason, &w.ReviewedBy, &w.ReviewedAt, &w.TransactionID,
				&w.TransferReceiptURL, &w.CreatedAt, &w.UpdatedAt,
			); err != nil {
				return err
			}
			w.Status = billing.WithdrawalStatus(statusStr)
			list = append(list, &w)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// AdminApproveWithdrawalRequest approves a pending withdrawal, writes the ledger debit transaction, and updates status.
func (r *Repository) AdminApproveWithdrawalRequest(
	ctx context.Context, withdrawalID int64, reviewerID int64, transferReceiptURL string,
) (*billing.WalletWithdrawal, *billing.WalletTransaction, error) {
	var w billing.WalletWithdrawal
	var txRecord *billing.WalletTransaction

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		queryWith := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payout_method_type, destination_details, user_payment_method_id, COALESCE(user_notes, ''),
			       status, created_at, updated_at
			FROM billing.wallet_withdrawals
			WHERE id = $1
			FOR UPDATE;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, queryWith, withdrawalID).Scan(
			&w.ID, &w.PublicID, &w.WalletID, &w.UserID, &w.OrganizationID, &w.Amount, &w.Currency,
			&w.PayoutMethodType, &w.DestinationDetails, &w.UserPaymentMethodID, &w.UserNotes,
			&statusStr, &w.CreatedAt, &w.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("withdrawal_request")
			}
			return err
		}
		w.Status = billing.WithdrawalStatus(statusStr)
		if w.Status != billing.WithdrawalPending {
			return apperr.Conflict("withdrawal.already_processed", fmt.Sprintf("طلب السحب تمت معالجته مسبقاً بحالة %s", w.Status))
		}

		// Check current balance and ensure no overdraft
		currentBalance, err := r.latestBalance(txCtx, tx, w.WalletID, true)
		if err != nil {
			return fmt.Errorf("read wallet balance: %w", err)
		}
		if currentBalance.Minor() < w.Amount.Minor() {
			return apperr.Validation("wallet.insufficient_funds", "رصيد المحفظة الحالي غير كافٍ لإتمام عملية السحب.", nil)
		}

		negDelta, err := money.Zero.Sub(w.Amount)
		if err != nil {
			return fmt.Errorf("compute negative delta: %w", err)
		}

		newBalance, err := currentBalance.Add(negDelta)
		if err != nil {
			return fmt.Errorf("compute updated wallet balance: %w", err)
		}

		desc := fmt.Sprintf("سحب رصيد معتمد إلى: %s", w.DestinationDetails)
		if w.UserNotes != "" {
			desc += " - " + w.UserNotes
		}

		queryInsertTx := `
			INSERT INTO billing.wallet_transactions (wallet_id, type, amount, balance_after, reference_type, reference_id, description)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, wallet_id, type, amount, balance_after, reference_type, reference_id, description, created_at;
		`
		var tRec billing.WalletTransaction
		var txTypeStr string
		if err := tx.QueryRow(
			txCtx, queryInsertTx,
			w.WalletID, billing.TxWithdrawal, negDelta, newBalance, "withdrawal_approval", w.ID, desc,
		).Scan(
			&tRec.ID, &tRec.WalletID, &txTypeStr, &tRec.Amount, &tRec.BalanceAfter,
			&tRec.ReferenceType, &tRec.ReferenceID, &tRec.Description, &tRec.CreatedAt,
		); err != nil {
			return fmt.Errorf("record wallet debit transaction: %w", err)
		}
		tRec.Type = billing.TransactionType(txTypeStr)
		txRecord = &tRec

		now := time.Now()
		queryUpdateWith := `
			UPDATE billing.wallet_withdrawals
			SET status = 'approved', reviewed_by = $1, reviewed_at = $2, transaction_id = $3,
			    transfer_receipt_url = CASE WHEN $4::text != '' THEN $4::text ELSE transfer_receipt_url END,
			    updated_at = now()
			WHERE id = $5
			RETURNING updated_at;
		`
		if err := tx.QueryRow(txCtx, queryUpdateWith, reviewerID, now, tRec.ID, transferReceiptURL, w.ID).Scan(&w.UpdatedAt); err != nil {
			return fmt.Errorf("update withdrawal status: %w", err)
		}
		w.Status = billing.WithdrawalApproved
		w.ReviewedBy = &reviewerID
		w.ReviewedAt = &now
		w.TransactionID = &tRec.ID
		if transferReceiptURL != "" {
			w.TransferReceiptURL = transferReceiptURL
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: w.OrganizationID,
			ActorUserID:    reviewerID,
			Action:         "commerce.withdrawal.approve",
			EntityType:     "wallet_withdrawal",
			EntityID:       strconv.FormatInt(w.ID, 10),
			Before:         map[string]any{"status": "pending"},
			After:          map[string]any{"status": "approved", "amount": w.Amount.String(), "tx_id": tRec.ID},
		})

		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &w, txRecord, nil
}

// AdminRejectWithdrawalRequest rejects a pending withdrawal and records the rejection reason.
func (r *Repository) AdminRejectWithdrawalRequest(
	ctx context.Context, withdrawalID int64, reviewerID int64, reason string,
) (*billing.WalletWithdrawal, error) {
	var w billing.WalletWithdrawal

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		queryWith := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payout_method_type, destination_details, user_payment_method_id, COALESCE(user_notes, ''),
			       status, created_at, updated_at
			FROM billing.wallet_withdrawals
			WHERE id = $1
			FOR UPDATE;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, queryWith, withdrawalID).Scan(
			&w.ID, &w.PublicID, &w.WalletID, &w.UserID, &w.OrganizationID, &w.Amount, &w.Currency,
			&w.PayoutMethodType, &w.DestinationDetails, &w.UserPaymentMethodID, &w.UserNotes,
			&statusStr, &w.CreatedAt, &w.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("withdrawal_request")
			}
			return err
		}
		w.Status = billing.WithdrawalStatus(statusStr)
		if w.Status != billing.WithdrawalPending {
			return apperr.Conflict("withdrawal.already_processed", fmt.Sprintf("طلب السحب تمت معالجته مسبقاً بحالة %s", w.Status))
		}

		now := time.Now()
		queryUpdateWith := `
			UPDATE billing.wallet_withdrawals
			SET status = 'rejected', reviewed_by = $1, reviewed_at = $2, rejection_reason = $3, updated_at = now()
			WHERE id = $4
			RETURNING updated_at;
		`
		if err := tx.QueryRow(txCtx, queryUpdateWith, reviewerID, now, reason, w.ID).Scan(&w.UpdatedAt); err != nil {
			return fmt.Errorf("update withdrawal status: %w", err)
		}
		w.Status = billing.WithdrawalRejected
		w.ReviewedBy = &reviewerID
		w.ReviewedAt = &now
		w.RejectionReason = reason

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: w.OrganizationID,
			ActorUserID:    reviewerID,
			Action:         "commerce.withdrawal.reject",
			EntityType:     "wallet_withdrawal",
			EntityID:       strconv.FormatInt(w.ID, 10),
			Before:         map[string]any{"status": "pending"},
			After:          map[string]any{"status": "rejected", "reason": reason},
		})

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &w, nil
}
