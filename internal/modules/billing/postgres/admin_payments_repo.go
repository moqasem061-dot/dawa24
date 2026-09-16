package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AdminListDetailedPayments returns payments enriched with order numbers and user/organization metadata.
func (r *Repository) AdminListDetailedPayments(ctx context.Context, filter billing.PaymentFilter) ([]*billing.AdminPaymentView, int, error) {
	var list []*billing.AdminPaymentView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		baseQuery := `
			FROM billing.payments p
			LEFT JOIN identity.users u ON p.user_id = u.id
			LEFT JOIN org.organizations org ON p.organization_id = org.id
			LEFT JOIN commerce.orders o ON p.order_id = o.id
			LEFT JOIN billing.invoices inv ON p.invoice_id = inv.id
			LEFT JOIN org.organizations cust ON COALESCE(inv.customer_org_id, o.organization_id) = cust.id
			WHERE 1=1
		`
		args := []any{}
		argIdx := 1

		if filter.OrganizationID != nil && *filter.OrganizationID > 0 {
			baseQuery += fmt.Sprintf(` AND (p.organization_id = $%d OR inv.organization_id = $%d OR cust.id = $%d)`, argIdx, argIdx, argIdx)
			args = append(args, *filter.OrganizationID)
			argIdx++
		}

		if filter.InvoiceID != nil && *filter.InvoiceID > 0 {
			baseQuery += fmt.Sprintf(` AND (p.invoice_id = $%d OR (p.invoice_id IS NULL AND p.order_id = inv.order_id AND inv.id = $%d))`, argIdx, argIdx)
			args = append(args, *filter.InvoiceID)
			argIdx++
		}

		if filter.Status != "" && filter.Status != "all" {
			baseQuery += fmt.Sprintf(` AND p.status = $%d`, argIdx)
			args = append(args, filter.Status)
			argIdx++
		}

		if filter.Method != "" && filter.Method != "all" {
			baseQuery += fmt.Sprintf(` AND p.method = $%d`, argIdx)
			args = append(args, filter.Method)
			argIdx++
		}

		if filter.DateFrom != "" {
			baseQuery += fmt.Sprintf(` AND COALESCE(p.paid_at, p.created_at)::date >= $%d::date`, argIdx)
			args = append(args, filter.DateFrom)
			argIdx++
		}

		if filter.DateTo != "" {
			baseQuery += fmt.Sprintf(` AND COALESCE(p.paid_at, p.created_at)::date <= $%d::date`, argIdx)
			args = append(args, filter.DateTo)
			argIdx++
		}

		if filter.Search != "" {
			searchPattern := "%" + strings.ToLower(filter.Search) + "%"
			baseQuery += fmt.Sprintf(` AND (
				LOWER(COALESCE(p.transaction_id, '')) LIKE $%d OR
				LOWER(COALESCE(p.reference_number, '')) LIKE $%d OR
				LOWER(COALESCE(inv.invoice_number, '')) LIKE $%d OR
				LOWER(COALESCE(cust.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(cust.trade_name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(u.name->>'ar', '')) LIKE $%d OR
				LOWER(u.email) LIKE $%d OR
				LOWER(COALESCE(org.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(o.order_number, '')) LIKE $%d
			)`, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx)
			args = append(args, searchPattern)
			argIdx++
		}

		if filter.CustomerSearch != "" {
			custPattern := "%" + strings.ToLower(filter.CustomerSearch) + "%"
			baseQuery += fmt.Sprintf(` AND (
				LOWER(COALESCE(cust.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(cust.trade_name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(cust.trade_name->>'en', '')) LIKE $%d OR
				LOWER(COALESCE(cust.name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(cust.name->>'en', '')) LIKE $%d OR
				LOWER(COALESCE(cust.organization_number, '')) LIKE $%d
			)`, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx)
			args = append(args, custPattern)
			argIdx++
		}

		countQuery := `SELECT COUNT(*) ` + baseQuery
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		selectQuery := `
			SELECT 
				p.id, p.public_id::text, p.payment_integration_id, p.order_id,
				COALESCE(o.order_number, ''),
				p.user_id,
				COALESCE(u.name->>'ar', u.name->>'en', u.email, 'مستخدم'),
				p.organization_id,
				COALESCE(org.legal_name, org.trade_name->>'ar', ''),
				p.amount, p.method, p.status, p.transaction_id, p.reference_number, p.paid_at, p.created_at,
				p.invoice_id,
				COALESCE(inv.invoice_number, ''),
				COALESCE(NULLIF(cust.name->>'ar', ''), NULLIF(cust.trade_name->>'ar', ''), NULLIF(cust.legal_name, ''), NULLIF(cust.name->>'en', ''), 'صيدلية معتمدة'),
				COALESCE(p.notes, '')
		` + baseQuery + fmt.Sprintf(` ORDER BY p.created_at DESC, p.id DESC LIMIT $%d OFFSET $%d;`, argIdx, argIdx+1)

		args = append(args, pageLimit(filter.Limit), pageOffset(filter.Offset))

		rows, err := tx.Query(txCtx, selectQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var pv billing.AdminPaymentView
			if err := rows.Scan(
				&pv.ID, &pv.PublicID, &pv.PaymentIntegrationID, &pv.OrderID,
				&pv.OrderNumber, &pv.UserID, &pv.UserName, &pv.OrganizationID,
				&pv.OrganizationName, &pv.Amount, &pv.Method, &pv.Status,
				&pv.TransactionID, &pv.ReferenceNumber, &pv.PaidAt, &pv.CreatedAt,
				&pv.InvoiceID, &pv.InvoiceNumber, &pv.CustomerName, &pv.Notes,
			); err != nil {
				return err
			}
			list = append(list, &pv)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminPerformWalletAdjustment posts a manual adjustment (deposit, withdrawal, or adjustment) to a wallet ledger.
func (r *Repository) AdminPerformWalletAdjustment(
	ctx context.Context,
	walletID int64,
	amount money.Amount,
	txType billing.TransactionType,
	reason string,
	actorID int64,
) error {
	if reason == "" {
		return apperr.Validation("wallet.reason_required", "A reason is required for a manual wallet adjustment.", nil)
	}
	if amount.Minor() == 0 {
		return apperr.Validation("wallet.zero_amount", "A wallet adjustment must be a non-zero amount.", nil)
	}
	if txType == "" {
		txType = billing.TxAdjustment
	}

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var ownerUserID int64
		if err := tx.QueryRow(txCtx,
			`SELECT user_id FROM billing.wallets WHERE id = $1;`, walletID,
		).Scan(&ownerUserID); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("wallet")
			}
			return fmt.Errorf("billing postgres: read wallet: %w", err)
		}

		current, err := r.latestBalance(txCtx, tx, walletID, true)
		if err != nil {
			return fmt.Errorf("billing postgres: read wallet balance: %w", err)
		}

		next, addErr := current.Add(amount)
		if addErr != nil {
			return apperr.Internal(addErr)
		}
		if next.IsNegative() {
			return apperr.Validation("wallet.insufficient_funds",
				"This adjustment would take the wallet below zero.", nil)
		}

		if _, err := tx.Exec(txCtx, `
			INSERT INTO billing.wallet_transactions (
				wallet_id, type, amount, balance_after, reference_type, description
			) VALUES ($1, $2, $3, $4, 'admin_adjustment', $5);
		`, walletID, string(txType), amount, next, reason); err != nil {
			return fmt.Errorf("billing postgres: record adjustment: %w", err)
		}

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      "billing.wallet.adjusted",
			EntityType:  "billing.wallet",
			EntityID:    strconv.FormatInt(walletID, 10),
			Before:      map[string]string{"balance": current.String()},
			After: map[string]string{
				"balance": next.String(),
				"amount":  amount.String(),
				"type":    string(txType),
				"reason":  reason,
			},
		})
	})
}

// AdminListSubscriptions returns subscriptions across all tenants.
func (r *Repository) AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*billing.Subscription, error) {
	subs, _, err := r.AdminListSubscriptionsWithTotal(ctx, limit, offset)
	return subs, err
}

// AdminListSubscriptionsWithTotal returns subscriptions across all tenants along with total count.
func (r *Repository) AdminListSubscriptionsWithTotal(ctx context.Context, limit, offset int) ([]*billing.Subscription, int, error) {
	var list []*billing.Subscription
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `SELECT count(*) FROM billing.subscriptions`).Scan(&total); err != nil {
			return err
		}

		const query = `
			SELECT id, public_id, user_id, organization_id, plan_id, status,
			       starts_at, expires_at, source_system, source_id, created_at, updated_at
			FROM billing.subscriptions
			ORDER BY created_at DESC, id DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err := tx.Query(txCtx, query, pageLimit(limit), pageOffset(offset))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s billing.Subscription
			var statusStr string
			if err := rows.Scan(
				&s.ID, &s.PublicID, &s.UserID, &s.OrganizationID, &s.PlanID, &statusStr,
				&s.StartsAt, &s.ExpiresAt, &s.SourceSystem, &s.SourceID,
				&s.CreatedAt, &s.UpdatedAt,
			); err != nil {
				return err
			}
			s.Status = billing.SubscriptionStatus(statusStr)
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminListPayments returns payments across all tenants.
func (r *Repository) AdminListPayments(ctx context.Context, limit, offset int) ([]*billing.Payment, error) {
	var list []*billing.Payment
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, payment_integration_id, order_id, user_id, organization_id,
			       amount, method, status, transaction_id, reference_number, paid_at,
			       created_at, updated_at
			FROM billing.payments
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err := tx.Query(txCtx, query, pageLimit(limit), pageOffset(offset))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p billing.Payment
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.PaymentIntegrationID, &p.OrderID, &p.UserID,
				&p.OrganizationID, &p.Amount, &p.Method, &p.Status, &p.TransactionID,
				&p.ReferenceNumber, &p.PaidAt, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &p)
		}
		return rows.Err()
	})
	return list, err
}

// AdminAdjustWallet posts a manual correction to a wallet ledger.
func (r *Repository) AdminAdjustWallet(
	ctx context.Context,
	walletID int64,
	amount money.Amount,
	reason string,
	actorID int64,
) error {
	return r.AdminPerformWalletAdjustment(ctx, walletID, amount, billing.TxAdjustment, reason, actorID)
}

// ListVendorCustomerOrgs returns distinct customer organizations that have transactions with this vendor.
func (r *Repository) ListVendorCustomerOrgs(ctx context.Context, vendorOrgID int64) ([]*billing.CustomerOrgSummary, error) {
	var list []*billing.CustomerOrgSummary
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT DISTINCT cust.id, 
			       COALESCE(NULLIF(cust.name->>'ar', ''), NULLIF(cust.trade_name->>'ar', ''), NULLIF(cust.legal_name, ''), NULLIF(cust.name->>'en', ''), 'صيدلية') AS name, 
			       COALESCE(cust.organization_number, '') AS code
			FROM org.organizations cust
			WHERE cust.id IN (
				SELECT inv.customer_org_id 
				FROM billing.invoices inv 
				WHERE inv.organization_id = $1 AND inv.customer_org_id IS NOT NULL
				UNION
				SELECT COALESCE(inv2.customer_org_id, o.organization_id)
				FROM billing.payments p
				LEFT JOIN billing.invoices inv2 ON p.invoice_id = inv2.id
				LEFT JOIN commerce.orders o ON p.order_id = o.id
				WHERE (p.organization_id = $1 OR inv2.organization_id = $1)
				  AND COALESCE(inv2.customer_org_id, o.organization_id) IS NOT NULL
				UNION
				SELECT o2.organization_id
				FROM commerce.order_shipments sh
				JOIN commerce.orders o2 ON sh.order_id = o2.id
				WHERE sh.organization_id = $1 AND o2.organization_id IS NOT NULL
			)
			ORDER BY name ASC;
		`
		rows, err := tx.Query(txCtx, query, vendorOrgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c billing.CustomerOrgSummary
			if err := rows.Scan(&c.ID, &c.Name, &c.Code); err != nil {
				return err
			}
			list = append(list, &c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("billing postgres: list vendor customer orgs: %w", err)
	}
	return list, nil
}
