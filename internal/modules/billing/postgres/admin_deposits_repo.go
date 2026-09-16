package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AdminListDetailedDeposits queries all deposit requests with complete user, organization, and review metadata.
func (r *Repository) AdminListDetailedDeposits(ctx context.Context, filter billing.DepositFilter) ([]*billing.AdminWalletDepositView, int, error) {
	var list []*billing.AdminWalletDepositView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		baseQuery := `
			FROM billing.wallet_deposits d
			JOIN identity.users u ON d.user_id = u.id
			LEFT JOIN billing.wallets w ON d.wallet_id = w.id
			LEFT JOIN org.organizations o ON o.id = COALESCE(
				d.organization_id,
				w.organization_id,
				(SELECT m.organization_id FROM org.members m WHERE m.user_id = d.user_id AND m.status = 'active' ORDER BY CASE WHEN m.role_key IN ('owner', 'org_owner') THEN 0 ELSE 1 END, m.id LIMIT 1),
				(SELECT org_owned.id FROM org.organizations org_owned WHERE org_owned.owner_id = d.user_id LIMIT 1)
			)
			LEFT JOIN identity.users rev ON d.reviewed_by = rev.id
			LEFT JOIN billing.wallet_transactions comp ON comp.reverses_transaction_id = d.transaction_id
			LEFT JOIN identity.users ref_u ON comp.refunded_by = ref_u.id
			WHERE 1=1
		`
		args := []any{}
		argIdx := 1

		if filter.Status != "" && filter.Status != "all" {
			baseQuery += fmt.Sprintf(` AND d.status = $%d`, argIdx)
			args = append(args, filter.Status)
			argIdx++
		}

		if filter.PaymentMethod != "" && filter.PaymentMethod != "all" {
			baseQuery += fmt.Sprintf(` AND d.payment_method = $%d`, argIdx)
			args = append(args, filter.PaymentMethod)
			argIdx++
		}

		if filter.WalletID > 0 {
			baseQuery += fmt.Sprintf(` AND d.wallet_id = $%d`, argIdx)
			args = append(args, filter.WalletID)
			argIdx++
		}

		if filter.OrganizationID > 0 {
			baseQuery += fmt.Sprintf(` AND (d.organization_id = $%d OR o.id = $%d)`, argIdx, argIdx)
			args = append(args, filter.OrganizationID)
			argIdx++
		}

		if filter.UserID > 0 {
			baseQuery += fmt.Sprintf(` AND d.user_id = $%d`, argIdx)
			args = append(args, filter.UserID)
			argIdx++
		}

		if filter.Search != "" {
			searchPattern := "%" + strings.ToLower(filter.Search) + "%"
			baseQuery += fmt.Sprintf(` AND (
				LOWER(d.reference_number) LIKE $%d OR
				LOWER(COALESCE(u.name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(u.name->>'en', '')) LIKE $%d OR
				LOWER(u.email) LIKE $%d OR
				LOWER(COALESCE(u.phone, '')) LIKE $%d OR
				LOWER(COALESCE(o.name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(o.name->>'en', '')) LIKE $%d OR
				LOWER(COALESCE(o.trade_name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(o.trade_name->>'en', '')) LIKE $%d OR
				LOWER(COALESCE(o.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(d.user_notes, '')) LIKE $%d
			)`, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx)
			args = append(args, searchPattern)
			argIdx++
		}

		countQuery := `SELECT COUNT(*) ` + baseQuery
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		selectQuery := `
			SELECT
				d.id,
				d.public_id::text,
				d.wallet_id,
				d.user_id,
				COALESCE(u.name->>'ar', u.name->>'en', u.email, 'مستخدم'),
				u.email,
				COALESCE(u.phone, ''),
				COALESCE(d.organization_id, o.id),
				COALESCE(NULLIF(o.name->>'ar', ''), NULLIF(o.trade_name->>'ar', ''), NULLIF(o.legal_name, ''), NULLIF(o.name->>'en', ''), NULLIF(o.trade_name->>'en', ''), ''),
				COALESCE(o.type, ''),
				d.amount,
				d.currency,
				d.payment_method,
				d.reference_number,
				COALESCE(d.attachment_url, ''),
				COALESCE(d.user_notes, ''),
				d.status,
				COALESCE(d.rejection_reason, ''),
				d.reviewed_by,
				COALESCE(rev.name->>'ar', rev.name->>'en', rev.email, ''),
				d.reviewed_at,
				d.transaction_id,
				(comp.id IS NOT NULL OR d.status = 'refunded') AS is_refunded,
				comp.refunded_by,
				COALESCE(ref_u.name->>'ar', ref_u.name->>'en', ref_u.email, ''),
				comp.created_at,
				comp.id,
				d.created_at,
				d.updated_at
		` + baseQuery + fmt.Sprintf(` ORDER BY d.created_at DESC LIMIT $%d OFFSET $%d;`, argIdx, argIdx+1)

		args = append(args, pageLimit(filter.Limit), pageOffset(filter.Offset))

		rows, err := tx.Query(txCtx, selectQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v billing.AdminWalletDepositView
			var statusStr string
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.WalletID, &v.UserID, &v.UserName, &v.UserEmail, &v.UserPhone,
				&v.OrganizationID, &v.OrganizationName, &v.OrganizationType,
				&v.Amount, &v.Currency, &v.PaymentMethod, &v.ReferenceNumber,
				&v.AttachmentURL, &v.UserNotes, &statusStr, &v.RejectionReason,
				&v.ReviewedBy, &v.ReviewerName, &v.ReviewedAt, &v.TransactionID,
				&v.IsRefunded, &v.RefundedBy, &v.RefundedByName, &v.RefundedAt, &v.RefundTransactionID,
				&v.CreatedAt, &v.UpdatedAt,
			); err != nil {
				return err
			}
			v.Status = billing.DepositStatus(statusStr)
			list = append(list, &v)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// AdminApproveDepositRequest approves a pending deposit, creates a ledger transaction, and updates the wallet balance.
func (r *Repository) AdminApproveDepositRequest(ctx context.Context, depositID int64, reviewerID int64) (*billing.WalletDeposit, *billing.WalletTransaction, error) {
	var dep billing.WalletDeposit
	var txRecord *billing.WalletTransaction

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		queryDep := `
			SELECT d.id, d.public_id::text, d.wallet_id, d.user_id,
			       COALESCE(d.organization_id, w.organization_id, (SELECT m.organization_id FROM org.members m WHERE m.user_id = d.user_id AND m.status = 'active' ORDER BY CASE WHEN m.role_key IN ('owner', 'org_owner') THEN 0 ELSE 1 END, m.id LIMIT 1), (SELECT org_owned.id FROM org.organizations org_owned WHERE org_owned.owner_id = d.user_id LIMIT 1)) AS organization_id,
			       d.amount, d.currency,
			       d.payment_method, d.reference_number, COALESCE(d.attachment_url, ''), COALESCE(d.user_notes, ''),
			       d.status, d.created_at, d.updated_at
			FROM billing.wallet_deposits d
			LEFT JOIN billing.wallets w ON d.wallet_id = w.id
			WHERE d.id = $1
			FOR UPDATE OF d;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, queryDep, depositID).Scan(
			&dep.ID, &dep.PublicID, &dep.WalletID, &dep.UserID, &dep.OrganizationID, &dep.Amount, &dep.Currency,
			&dep.PaymentMethod, &dep.ReferenceNumber, &dep.AttachmentURL, &dep.UserNotes,
			&statusStr, &dep.CreatedAt, &dep.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("deposit_request")
			}
			return err
		}
		dep.Status = billing.DepositStatus(statusStr)
		if dep.Status != billing.DepositPending {
			return apperr.Conflict("deposit.already_processed", fmt.Sprintf(i18n.TDefault("w4_mod.s_71"), dep.Status))
		}

		currentBalance, err := r.latestBalance(txCtx, tx, dep.WalletID, true)
		if err != nil {
			return fmt.Errorf("read wallet balance: %w", err)
		}

		newBalance, err := currentBalance.Add(dep.Amount)
		if err != nil {
			return fmt.Errorf("compute updated wallet balance: %w", err)
		}

		desc := fmt.Sprintf(i18n.TDefault("w4_mod.s_s_72"), dep.PaymentMethod, dep.ReferenceNumber)
		if dep.UserNotes != "" {
			desc += " - " + dep.UserNotes
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
			dep.WalletID, billing.TxDeposit, dep.Amount, newBalance, "deposit_approval", dep.ID, desc,
		).Scan(
			&tRec.ID, &tRec.WalletID, &txTypeStr, &tRec.Amount, &tRec.BalanceAfter,
			&tRec.ReferenceType, &tRec.ReferenceID, &tRec.Description, &tRec.CreatedAt,
		); err != nil {
			return fmt.Errorf("record wallet transaction: %w", err)
		}
		tRec.Type = billing.TransactionType(txTypeStr)
		txRecord = &tRec

		now := time.Now()
		queryUpdateDep := `
			UPDATE billing.wallet_deposits
			SET status = 'approved', reviewed_by = $1, reviewed_at = $2, transaction_id = $3, organization_id = COALESCE(organization_id, $4), updated_at = now()
			WHERE id = $5
			RETURNING updated_at;
		`
		if err := tx.QueryRow(txCtx, queryUpdateDep, reviewerID, now, tRec.ID, dep.OrganizationID, dep.ID).Scan(&dep.UpdatedAt); err != nil {
			return fmt.Errorf("update deposit status: %w", err)
		}
		dep.Status = billing.DepositApproved
		dep.ReviewedBy = &reviewerID
		dep.ReviewedAt = &now
		dep.TransactionID = &tRec.ID

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: dep.OrganizationID,
			ActorUserID:    reviewerID,
			Action:         "commerce.deposit.approve",
			EntityType:     "wallet_deposit",
			EntityID:       strconv.FormatInt(dep.ID, 10),
			Before:         map[string]any{"status": "pending"},
			After:          map[string]any{"status": "approved", "amount": dep.Amount.String(), "tx_id": tRec.ID},
		})

		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &dep, txRecord, nil
}

// AdminRejectDepositRequest rejects a pending deposit and records the rejection reason.
func (r *Repository) AdminRejectDepositRequest(ctx context.Context, depositID int64, reviewerID int64, reason string) (*billing.WalletDeposit, error) {
	var dep billing.WalletDeposit

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		queryDep := `
			SELECT d.id, d.public_id::text, d.wallet_id, d.user_id,
			       COALESCE(d.organization_id, w.organization_id, (SELECT m.organization_id FROM org.members m WHERE m.user_id = d.user_id AND m.status = 'active' ORDER BY CASE WHEN m.role_key IN ('owner', 'org_owner') THEN 0 ELSE 1 END, m.id LIMIT 1), (SELECT org_owned.id FROM org.organizations org_owned WHERE org_owned.owner_id = d.user_id LIMIT 1)) AS organization_id,
			       d.amount, d.currency,
			       d.payment_method, d.reference_number, COALESCE(d.attachment_url, ''), COALESCE(d.user_notes, ''),
			       d.status, d.created_at, d.updated_at
			FROM billing.wallet_deposits d
			LEFT JOIN billing.wallets w ON d.wallet_id = w.id
			WHERE id = $1
			FOR UPDATE OF d;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, queryDep, depositID).Scan(
			&dep.ID, &dep.PublicID, &dep.WalletID, &dep.UserID, &dep.OrganizationID, &dep.Amount, &dep.Currency,
			&dep.PaymentMethod, &dep.ReferenceNumber, &dep.AttachmentURL, &dep.UserNotes,
			&statusStr, &dep.CreatedAt, &dep.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("deposit_request")
			}
			return err
		}
		dep.Status = billing.DepositStatus(statusStr)
		if dep.Status != billing.DepositPending {
			return apperr.Conflict("deposit.already_processed", fmt.Sprintf(i18n.TDefault("w4_mod.s_71"), dep.Status))
		}

		now := time.Now()
		queryUpdateDep := `
			UPDATE billing.wallet_deposits
			SET status = 'rejected', rejection_reason = $1, reviewed_by = $2, reviewed_at = $3, organization_id = COALESCE(organization_id, $4), updated_at = now()
			WHERE id = $5
			RETURNING updated_at;
		`
		if err := tx.QueryRow(txCtx, queryUpdateDep, reason, reviewerID, now, dep.OrganizationID, dep.ID).Scan(&dep.UpdatedAt); err != nil {
			return fmt.Errorf("update deposit status: %w", err)
		}
		dep.Status = billing.DepositRejected
		dep.RejectionReason = reason
		dep.ReviewedBy = &reviewerID
		dep.ReviewedAt = &now

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: dep.OrganizationID,
			ActorUserID:    reviewerID,
			Action:         "commerce.deposit.reject",
			EntityType:     "wallet_deposit",
			EntityID:       strconv.FormatInt(dep.ID, 10),
			Before:         map[string]any{"status": "pending"},
			After:          map[string]any{"status": "rejected", "reason": reason},
		})

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &dep, nil
}

func pageLimit(limit int) int {
	if limit <= 0 || limit > 200 {
		return 50
	}
	return limit
}

func pageOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
