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

// AdminRefundTransaction creates a compensating refund transaction in the wallet ledger,
// credits the organization's wallet balance, links to the original transaction via reverses_transaction_id,
// and ensures idempotency and audit logging inside a single transaction.
func (r *Repository) AdminRefundTransaction(
	ctx context.Context,
	transactionID int64,
	reason string,
	actorID int64,
) (*billing.WalletTransaction, error) {
	if transactionID <= 0 {
		return nil, apperr.Validation("transaction.invalid_id", "Invalid transaction ID.", nil)
	}
	if reason == "" {
		return nil, apperr.Validation("refund.reason_required", "A reason is required to issue a refund.", nil)
	}

	var refundRecord *billing.WalletTransaction

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Fetch original transaction FOR UPDATE
		var (
			origID                int64
			walletID              int64
			origTypeStr           string
			origAmount            money.Amount
			origBalanceAfter      money.Amount
			origRefType           *string
			origRefID             *int64
			origDesc              *string
			origCreatedAt         time.Time
			reversesTransactionID *int64
		)

		err := tx.QueryRow(txCtx, `
			SELECT id, wallet_id, type, amount, balance_after, reference_type, reference_id, description, created_at, reverses_transaction_id
			FROM billing.wallet_transactions
			WHERE id = $1
			FOR UPDATE;
		`, transactionID).Scan(
			&origID, &walletID, &origTypeStr, &origAmount, &origBalanceAfter,
			&origRefType, &origRefID, &origDesc, &origCreatedAt, &reversesTransactionID,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("wallet_transaction")
			}
			return fmt.Errorf("read original transaction: %w", err)
		}

		// 2. Validate original transaction eligibility
		origType := billing.TransactionType(origTypeStr)
		if origType == billing.TxRefund {
			return apperr.Validation("transaction.cannot_refund_refund", "Cannot refund a refund transaction.", nil)
		}

		// 3. Check idempotency: ensure this transaction has not already been refunded
		var existingRefundID int64
		err = tx.QueryRow(txCtx, `
			SELECT id FROM billing.wallet_transactions
			WHERE reverses_transaction_id = $1
			LIMIT 1;
		`, transactionID).Scan(&existingRefundID)
		if err == nil {
			return apperr.Conflict("transaction.already_refunded", fmt.Sprintf("This transaction #%d has already been refunded by transaction #%d.", transactionID, existingRefundID))
		} else if !database.IsNotFound(err) {
			return fmt.Errorf("check existing refund: %w", err)
		}

		// 4. Determine refund credit amount:
		// Refunding always credits the wallet by the transaction value
		refundAmount := origAmount
		if refundAmount.IsNegative() {
			refundAmount = money.FromMinor(-refundAmount.Minor())
		}
		if refundAmount.Minor() <= 0 {
			return apperr.Validation("refund.zero_amount", "Original transaction has zero amount and cannot be refunded.", nil)
		}

		// 5. Read current wallet balance FOR UPDATE to prevent balance drift
		currentBalance, err := r.latestBalance(txCtx, tx, walletID, true)
		if err != nil {
			return fmt.Errorf("read current wallet balance: %w", err)
		}

		nextBalance, addErr := currentBalance.Add(refundAmount)
		if addErr != nil {
			return apperr.Internal(addErr)
		}

		// 6. Insert compensating refund ledger row
		refundDesc := fmt.Sprintf("استرداد معاملة #TX-%d: %s", transactionID, reason)
		var tRec billing.WalletTransaction
		var txTypeStr string
		var refBy *int64
		if actorID > 0 {
			refBy = &actorID
		}

		err = tx.QueryRow(txCtx, `
			INSERT INTO billing.wallet_transactions (
				wallet_id, type, amount, balance_after, reference_type, reference_id, description,
				reverses_transaction_id, refunded_by, refunded_at, created_at
			) VALUES (
				$1, 'refund', $2, $3, 'refund', $4, $5,
				$6, $7, now(), now()
			)
			RETURNING id, wallet_id, type, amount, balance_after, reference_type, reference_id, description, created_at,
			          reverses_transaction_id, refunded_by, refunded_at;
		`, walletID, refundAmount, nextBalance, transactionID, refundDesc, transactionID, refBy).Scan(
			&tRec.ID, &tRec.WalletID, &txTypeStr, &tRec.Amount, &tRec.BalanceAfter,
			&tRec.ReferenceType, &tRec.ReferenceID, &tRec.Description, &tRec.CreatedAt,
			&tRec.ReversesTransactionID, &tRec.RefundedBy, &tRec.RefundedAt,
		)
		if err != nil {
			return fmt.Errorf("insert compensating refund transaction: %w", err)
		}
		tRec.Type = billing.TransactionType(txTypeStr)
		refundRecord = &tRec

		// 7. If original was associated with a deposit request, update deposit request status
		if origRefType != nil && *origRefType == "deposit_approval" && origRefID != nil {
			_, _ = tx.Exec(txCtx, `
				UPDATE billing.wallet_deposits
				SET status = 'refunded', refunded_by = $1, refunded_at = now(), refund_transaction_id = $2, updated_at = now()
				WHERE id = $3;
			`, refBy, tRec.ID, *origRefID)
		} else {
			// Check if any deposit references this transaction
			_, _ = tx.Exec(txCtx, `
				UPDATE billing.wallet_deposits
				SET status = 'refunded', refunded_by = $1, refunded_at = now(), refund_transaction_id = $2, updated_at = now()
				WHERE transaction_id = $3;
			`, refBy, tRec.ID, transactionID)
		}

		// 8. Write audit row (WO-17)
		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      "billing.wallet.refunded",
			EntityType:  "billing.wallet_transaction",
			EntityID:    strconv.FormatInt(transactionID, 10),
			Before: map[string]string{
				"balance":        currentBalance.String(),
				"transaction_id": strconv.FormatInt(transactionID, 10),
				"type":           string(origType),
				"amount":         origAmount.String(),
			},
			After: map[string]string{
				"balance":               nextBalance.String(),
				"refund_amount":         refundAmount.String(),
				"refund_transaction_id": strconv.FormatInt(tRec.ID, 10),
				"reverses_transaction":  strconv.FormatInt(transactionID, 10),
				"reason":                reason,
			},
		})
	})

	if err != nil {
		return nil, err
	}
	return refundRecord, nil
}

// AdminRefundDeposit refunds an approved deposit request and creates a compensating refund transaction.
func (r *Repository) AdminRefundDeposit(
	ctx context.Context,
	depositID int64,
	reason string,
	actorID int64,
) (*billing.WalletDeposit, *billing.WalletTransaction, error) {
	if depositID <= 0 {
		return nil, nil, apperr.Validation("deposit.invalid_id", "Invalid deposit request ID.", nil)
	}
	if reason == "" {
		return nil, nil, apperr.Validation("refund.reason_required", "A reason is required to issue a refund.", nil)
	}

	var dep billing.WalletDeposit
	var txID *int64

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var statusStr string
		err := tx.QueryRow(txCtx, `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payment_method, reference_number, status, transaction_id, created_at, updated_at
			FROM billing.wallet_deposits
			WHERE id = $1;
		`, depositID).Scan(
			&dep.ID, &dep.PublicID, &dep.WalletID, &dep.UserID, &dep.OrganizationID, &dep.Amount, &dep.Currency,
			&dep.PaymentMethod, &dep.ReferenceNumber, &statusStr, &txID, &dep.CreatedAt, &dep.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("deposit_request")
			}
			return err
		}
		dep.Status = billing.DepositStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	if dep.Status == billing.DepositRefunded {
		return nil, nil, apperr.Conflict("deposit.already_refunded", "This deposit has already been refunded.")
	}
	if dep.Status != billing.DepositApproved {
		return nil, nil, apperr.Validation("deposit.not_approved", "Only approved deposits can be refunded.", nil)
	}
	if txID == nil || *txID <= 0 {
		return nil, nil, apperr.Validation("deposit.no_transaction", "This deposit has no associated transaction record.", nil)
	}

	// Refund the associated transaction
	refundTx, err := r.AdminRefundTransaction(ctx, *txID, reason, actorID)
	if err != nil {
		return nil, nil, err
	}

	dep.Status = billing.DepositRefunded
	dep.RefundTransactionID = &refundTx.ID
	dep.RefundedBy = &actorID
	now := time.Now()
	dep.RefundedAt = &now

	return &dep, refundTx, nil
}
