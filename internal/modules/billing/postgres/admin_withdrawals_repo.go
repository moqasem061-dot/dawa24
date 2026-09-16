package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// AdminListDetailedWithdrawals lists withdrawal requests with user and org context for admin review.
func (r *Repository) AdminListDetailedWithdrawals(
	ctx context.Context, filter billing.WithdrawalFilter,
) ([]*billing.AdminWalletWithdrawalView, int, error) {
	var list []*billing.AdminWalletWithdrawalView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		baseQuery := `
			FROM billing.wallet_withdrawals w
			LEFT JOIN identity.users u ON w.user_id = u.id
			LEFT JOIN billing.wallets wlt ON w.wallet_id = wlt.id
			LEFT JOIN org.organizations o ON o.id = COALESCE(
				w.organization_id,
				wlt.organization_id,
				(SELECT m.organization_id FROM org.members m WHERE m.user_id = w.user_id AND m.status = 'active' ORDER BY CASE WHEN m.role_key IN ('owner', 'org_owner') THEN 0 ELSE 1 END, m.id LIMIT 1),
				(SELECT org_owned.id FROM org.organizations org_owned WHERE org_owned.owner_id = w.user_id LIMIT 1)
			)
			LEFT JOIN identity.users rev ON w.reviewed_by = rev.id
			WHERE 1=1
		`
		args := []any{}
		argIdx := 1

		if filter.UserID > 0 {
			baseQuery += fmt.Sprintf(` AND w.user_id = $%d`, argIdx)
			args = append(args, filter.UserID)
			argIdx++
		}
		if filter.WalletID > 0 {
			baseQuery += fmt.Sprintf(` AND w.wallet_id = $%d`, argIdx)
			args = append(args, filter.WalletID)
			argIdx++
		}
		if filter.OrganizationID > 0 {
			baseQuery += fmt.Sprintf(` AND (w.organization_id = $%d OR o.id = $%d)`, argIdx, argIdx)
			args = append(args, filter.OrganizationID)
			argIdx++
		}
		if filter.Status != "" && filter.Status != "all" {
			baseQuery += fmt.Sprintf(` AND w.status = $%d`, argIdx)
			args = append(args, filter.Status)
			argIdx++
		}
		if filter.PayoutMethodType != "" && filter.PayoutMethodType != "all" {
			baseQuery += fmt.Sprintf(` AND w.payout_method_type = $%d`, argIdx)
			args = append(args, filter.PayoutMethodType)
			argIdx++
		}
		if filter.Search != "" {
			searchPattern := "%" + strings.ToLower(filter.Search) + "%"
			baseQuery += fmt.Sprintf(` AND (
				LOWER(w.destination_details) LIKE $%d OR
				LOWER(COALESCE(u.name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(u.name->>'en', '')) LIKE $%d OR
				LOWER(u.email) LIKE $%d OR
				LOWER(COALESCE(u.phone, '')) LIKE $%d OR
				LOWER(COALESCE(o.name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(o.name->>'en', '')) LIKE $%d OR
				LOWER(COALESCE(o.trade_name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(o.trade_name->>'en', '')) LIKE $%d OR
				LOWER(COALESCE(o.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(w.user_notes, '')) LIKE $%d
			)`, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx)
			args = append(args, searchPattern)
			argIdx++
		}

		countQuery := `SELECT COUNT(*) ` + baseQuery
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return fmt.Errorf("count withdrawals: %w", err)
		}

		limit := filter.Limit
		if limit <= 0 {
			limit = 20
		}

		selectQuery := fmt.Sprintf(`
			SELECT
				w.id,
				w.public_id::text,
				w.wallet_id,
				w.user_id,
				COALESCE(u.name->>'ar', u.name->>'en', u.email, 'مستخدم') AS user_name,
				COALESCE(u.email, '') AS user_email,
				COALESCE(u.phone, '') AS user_phone,
				COALESCE(w.organization_id, o.id),
				COALESCE(NULLIF(o.name->>'ar', ''), NULLIF(o.trade_name->>'ar', ''), NULLIF(o.legal_name, ''), NULLIF(o.name->>'en', ''), NULLIF(o.trade_name->>'en', ''), '') AS org_name,
				COALESCE(o.type, '') AS org_type,
				w.amount,
				w.currency,
				w.payout_method_type,
				w.destination_details,
				w.user_payment_method_id,
				COALESCE(w.user_notes, '') AS user_notes,
				w.status,
				COALESCE(w.rejection_reason, '') AS rejection_reason,
				w.reviewed_by,
				COALESCE(rev.name->>'ar', rev.name->>'en', rev.email, '') AS reviewer_name,
				w.reviewed_at,
				w.transaction_id,
				COALESCE(w.transfer_receipt_url, '') AS transfer_receipt_url,
				w.created_at,
				w.updated_at
			%%s
			ORDER BY w.created_at DESC
			LIMIT $%d OFFSET $%d;
		`, argIdx, argIdx+1)
		selectQuery = fmt.Sprintf(selectQuery, baseQuery)

		args = append(args, limit, filter.Offset)

		rows, err := tx.Query(txCtx, selectQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v billing.AdminWalletWithdrawalView
			var statusStr string
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.WalletID, &v.UserID,
				&v.UserName, &v.UserEmail, &v.UserPhone,
				&v.OrganizationID, &v.OrganizationName, &v.OrganizationType,
				&v.Amount, &v.Currency, &v.PayoutMethodType, &v.DestinationDetails, &v.UserPaymentMethodID,
				&v.UserNotes, &statusStr, &v.RejectionReason,
				&v.ReviewedBy, &v.ReviewerName, &v.ReviewedAt, &v.TransactionID,
				&v.TransferReceiptURL, &v.CreatedAt, &v.UpdatedAt,
			); err != nil {
				return err
			}
			v.Status = billing.WithdrawalStatus(statusStr)
			list = append(list, &v)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}
