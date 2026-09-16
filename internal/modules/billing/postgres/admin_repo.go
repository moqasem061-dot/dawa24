package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// EnsureAllOrgWallets guarantees that every registered organization has an active wallet account.
func (r *Repository) EnsureAllOrgWallets(ctx context.Context) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const queryOwners = `
			INSERT INTO billing.wallets (user_id, organization_id, currency)
			SELECT o.owner_id, o.id, 'EGP'
			FROM org.organizations o
			WHERE o.owner_id IS NOT NULL AND o.owner_id > 0
			ON CONFLICT (user_id, currency) DO UPDATE SET organization_id = EXCLUDED.organization_id;
		`
		if _, err := tx.Exec(txCtx, queryOwners); err != nil {
			return fmt.Errorf("seed org owner wallets: %w", err)
		}
		return nil
	})
}

// AdminListDetailedWallets returns all user/organization wallets enriched with user and organization details.
func (r *Repository) AdminListDetailedWallets(ctx context.Context, filter billing.WalletFilter) ([]*billing.AdminWalletView, int, error) {
	// Auto-seed missing wallets first
	_ = r.EnsureAllOrgWallets(ctx)

	var list []*billing.AdminWalletView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		baseQuery := `
			FROM billing.wallets w
			JOIN identity.users u ON w.user_id = u.id
			LEFT JOIN org.organizations o ON (w.organization_id = o.id OR (w.organization_id IS NULL AND o.owner_id = u.id))
			WHERE 1=1
		`
		args := []any{}
		argIdx := 1

		if filter.Search != "" {
			searchPattern := "%" + strings.ToLower(filter.Search) + "%"
			baseQuery += fmt.Sprintf(` AND (
				LOWER(COALESCE(u.name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(u.name->>'en', '')) LIKE $%d OR
				LOWER(u.email) LIKE $%d OR
				LOWER(COALESCE(u.phone, '')) LIKE $%d OR
				LOWER(COALESCE(o.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(o.trade_name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(o.trade_name->>'en', '')) LIKE $%d
			)`, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx)
			args = append(args, searchPattern)
			argIdx++
		}

		if filter.Type != "" && filter.Type != "all" {
			baseQuery += fmt.Sprintf(` AND o.type = $%d`, argIdx)
			args = append(args, filter.Type)
			argIdx++
		}

		if filter.OrganizationID > 0 {
			baseQuery += fmt.Sprintf(` AND (w.organization_id = $%d OR o.id = $%d)`, argIdx, argIdx)
			args = append(args, filter.OrganizationID)
			argIdx++
		}

		countQuery := `SELECT COUNT(*) ` + baseQuery
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		selectQuery := `
			SELECT 
				w.id, 
				w.public_id::text, 
				w.user_id, 
				COALESCE(u.name->>'ar', u.name->>'en', u.email, 'مستخدم'), 
				u.email, 
				COALESCE(u.phone, ''),
				w.organization_id,
				COALESCE(o.legal_name, o.trade_name->>'ar', o.trade_name->>'en', ''),
				COALESCE(o.type, ''),
				w.currency,
				w.created_at,
				(SELECT COUNT(*) FROM billing.wallet_transactions wt WHERE wt.wallet_id = w.id),
				COALESCE((SELECT balance_after FROM billing.wallet_transactions wt WHERE wt.wallet_id = w.id ORDER BY id DESC LIMIT 1), 0.00) AS balance,
				COALESCE((SELECT SUM(amount) FROM billing.wallet_withdrawals ww WHERE ww.wallet_id = w.id AND ww.status = 'pending'), 0.00) AS pending_withdrawal
		` + baseQuery + fmt.Sprintf(` ORDER BY w.created_at DESC LIMIT $%d OFFSET $%d;`, argIdx, argIdx+1)

		args = append(args, pageLimit(filter.Limit), pageOffset(filter.Offset))

		rows, err := tx.Query(txCtx, selectQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var wv billing.AdminWalletView
			if err := rows.Scan(
				&wv.ID, &wv.PublicID, &wv.UserID, &wv.UserName, &wv.UserEmail, &wv.UserPhone,
				&wv.OrganizationID, &wv.OrganizationName, &wv.OrganizationType, &wv.Currency,
				&wv.CreatedAt, &wv.TransactionsCount, &wv.Balance, &wv.PendingWithdrawal,
			); err != nil {
				return err
			}
			availMinor := wv.Balance.Minor() - wv.PendingWithdrawal.Minor()
			if availMinor < 0 {
				availMinor = 0
			}
			wv.AvailableBalance = money.FromMinor(availMinor)
			list = append(list, &wv)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminListDetailedTransactions returns wallet transactions enriched with user and organization metadata.
func (r *Repository) AdminListDetailedTransactions(ctx context.Context, filter billing.TransactionFilter) ([]*billing.AdminWalletTransactionView, int, error) {
	var list []*billing.AdminWalletTransactionView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		baseQuery := `
			FROM billing.wallet_transactions wt
			JOIN billing.wallets w ON wt.wallet_id = w.id
			JOIN identity.users u ON w.user_id = u.id
			LEFT JOIN org.organizations o ON w.organization_id = o.id
			LEFT JOIN billing.wallet_transactions comp ON comp.reverses_transaction_id = wt.id
			LEFT JOIN identity.users ref_u ON comp.refunded_by = ref_u.id
			WHERE 1=1
		`
		args := []any{}
		argIdx := 1

		if filter.WalletID > 0 {
			baseQuery += fmt.Sprintf(` AND wt.wallet_id = $%d`, argIdx)
			args = append(args, filter.WalletID)
			argIdx++
		}

		if filter.OrganizationID > 0 {
			baseQuery += fmt.Sprintf(` AND (w.organization_id = $%d OR o.id = $%d)`, argIdx, argIdx)
			args = append(args, filter.OrganizationID)
			argIdx++
		}

		if filter.Type != "" && filter.Type != "all" {
			baseQuery += fmt.Sprintf(` AND wt.type = $%d`, argIdx)
			args = append(args, filter.Type)
			argIdx++
		}

		if filter.Search != "" {
			searchPattern := "%" + strings.ToLower(filter.Search) + "%"
			baseQuery += fmt.Sprintf(` AND (
				LOWER(COALESCE(wt.description, '')) LIKE $%d OR
				LOWER(COALESCE(u.name->>'ar', '')) LIKE $%d OR
				LOWER(u.email) LIKE $%d OR
				LOWER(COALESCE(o.legal_name, '')) LIKE $%d
			)`, argIdx, argIdx, argIdx, argIdx)
			args = append(args, searchPattern)
			argIdx++
		}

		countQuery := `SELECT COUNT(*) ` + baseQuery
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		selectQuery := `
			SELECT 
				wt.id,
				wt.wallet_id,
				wt.type,
				wt.amount,
				wt.balance_after,
				COALESCE(wt.reference_type, ''),
				wt.reference_id,
				COALESCE(wt.description, ''),
				wt.created_at,
				w.user_id,
				COALESCE(u.name->>'ar', u.name->>'en', u.email, 'مستخدم'),
				u.email,
				COALESCE(o.legal_name, o.trade_name->>'ar', o.trade_name->>'en', ''),
				COALESCE(o.type, ''),
				wt.reverses_transaction_id,
				(comp.id IS NOT NULL) AS is_refunded,
				comp.refunded_by,
				COALESCE(ref_u.name->>'ar', ref_u.name->>'en', ref_u.email, ''),
				comp.created_at,
				comp.id
		` + baseQuery + fmt.Sprintf(` ORDER BY wt.id DESC LIMIT $%d OFFSET $%d;`, argIdx, argIdx+1)

		args = append(args, pageLimit(filter.Limit), pageOffset(filter.Offset))

		rows, err := tx.Query(txCtx, selectQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var tv billing.AdminWalletTransactionView
			var typeStr string
			if err := rows.Scan(
				&tv.ID, &tv.WalletID, &typeStr, &tv.Amount, &tv.BalanceAfter,
				&tv.ReferenceType, &tv.ReferenceID, &tv.Description, &tv.CreatedAt,
				&tv.UserID, &tv.UserName, &tv.UserEmail, &tv.OrganizationName, &tv.OrganizationType,
				&tv.ReversesTransactionID, &tv.IsRefunded, &tv.RefundedByID, &tv.RefundedByName,
				&tv.RefundedAt, &tv.RefundTransactionID,
			); err != nil {
				return err
			}
			tv.Type = billing.TransactionType(typeStr)
			list = append(list, &tv)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminListDetailedInvoices returns invoices enriched with party legal names and order details.
func (r *Repository) AdminListDetailedInvoices(ctx context.Context, filter billing.InvoiceFilter) ([]*billing.AdminInvoiceView, int, error) {
	var list []*billing.AdminInvoiceView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		baseQuery := `
			FROM billing.invoices inv
			LEFT JOIN org.organizations v ON inv.organization_id = v.id
			LEFT JOIN org.organizations c ON inv.customer_org_id = c.id
			LEFT JOIN commerce.orders o ON inv.order_id = o.id
			WHERE 1=1
		`
		args := []any{}
		argIdx := 1

		if filter.Status != "" && filter.Status != "all" {
			baseQuery += fmt.Sprintf(` AND inv.status = $%d`, argIdx)
			args = append(args, filter.Status)
			argIdx++
		}

		if filter.OrganizationID != nil && *filter.OrganizationID > 0 {
			if filter.CustomerOrgID != nil && *filter.CustomerOrgID > 0 {
				baseQuery += fmt.Sprintf(` AND inv.organization_id = $%d AND inv.customer_org_id = $%d`, argIdx, argIdx+1)
				args = append(args, *filter.OrganizationID, *filter.CustomerOrgID)
				argIdx += 2
			} else {
				baseQuery += fmt.Sprintf(` AND (inv.organization_id = $%d OR inv.customer_org_id = $%d)`, argIdx, argIdx)
				args = append(args, *filter.OrganizationID)
				argIdx++
			}
		} else if filter.CustomerOrgID != nil && *filter.CustomerOrgID > 0 {
			baseQuery += fmt.Sprintf(` AND inv.customer_org_id = $%d`, argIdx)
			args = append(args, *filter.CustomerOrgID)
			argIdx++
		}

		if filter.BranchID != nil && *filter.BranchID > 0 {
			baseQuery += fmt.Sprintf(` AND (o.vendor_branch_id = $%d OR o.branch_id = $%d)`, argIdx, argIdx)
			args = append(args, *filter.BranchID)
			argIdx++
		}

		if filter.Search != "" {
			searchPattern := "%" + strings.ToLower(filter.Search) + "%"
			baseQuery += fmt.Sprintf(` AND (
				LOWER(inv.invoice_number) LIKE $%d OR
				LOWER(COALESCE(v.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(c.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(o.order_number, '')) LIKE $%d
			)`, argIdx, argIdx, argIdx, argIdx)
			args = append(args, searchPattern)
			argIdx++
		}

		if filter.DateFrom != "" {
			baseQuery += fmt.Sprintf(` AND inv.issue_date >= $%d::date`, argIdx)
			args = append(args, filter.DateFrom)
			argIdx++
		}
		if filter.DateTo != "" {
			baseQuery += fmt.Sprintf(` AND inv.issue_date <= ($%d::date + INTERVAL '1 day')`, argIdx)
			args = append(args, filter.DateTo)
			argIdx++
		}

		countQuery := `SELECT COUNT(*) ` + baseQuery
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		orderClause := "ORDER BY inv.created_at DESC, inv.id DESC"
		sortCol := ""
		switch filter.SortBy {
		case "invoice_number":
			sortCol = "inv.invoice_number"
		case "order_number", "order_id":
			sortCol = "COALESCE(o.order_number, '')"
		case "total", "total_amount":
			sortCol = "inv.total_amount"
		case "paid", "paid_amount":
			sortCol = "paid_amount"
		case "status":
			sortCol = "inv.status"
		case "due_date":
			sortCol = "inv.due_date"
		case "issue_date":
			sortCol = "inv.issue_date"
		case "customer", "customer_name":
			sortCol = "COALESCE(c.legal_name, c.trade_name->>'ar', '')"
		case "vendor", "vendor_name":
			sortCol = "COALESCE(v.legal_name, v.trade_name->>'ar', '')"
		case "date", "created_at":
			sortCol = "inv.created_at"
		}
		if sortCol != "" {
			dir := "ASC"
			if strings.EqualFold(filter.SortOrder, "desc") {
				dir = "DESC"
			}
			orderClause = fmt.Sprintf("ORDER BY %s %s, inv.id DESC", sortCol, dir)
		}

		selectQuery := `
			SELECT 
				inv.id, inv.public_id::text, inv.organization_id,
				COALESCE(v.legal_name, v.trade_name->>'ar', 'المورد'),
				inv.customer_org_id,
				COALESCE(c.legal_name, c.trade_name->>'ar', 'الصيدلية'),
				inv.order_id,
				COALESCE(o.order_number, ''),
				inv.invoice_number,
				inv.issue_date,
				inv.due_date,
				inv.subtotal,
				inv.tax_amount,
				inv.discount_amount,
				inv.total_amount,
				inv.status,
				inv.payment_method,
				COALESCE(inv.notes, ''),
				inv.created_at,
				COALESCE((SELECT SUM(p.amount) FROM billing.payments p WHERE (p.invoice_id = inv.id OR (p.invoice_id IS NULL AND p.order_id IS NOT NULL AND p.order_id = inv.order_id)) AND p.status IN ('paid', 'completed')), 0) AS paid_amount
		` + baseQuery + fmt.Sprintf(` %s LIMIT $%d OFFSET $%d;`, orderClause, argIdx, argIdx+1)

		args = append(args, pageLimit(filter.Limit), pageOffset(filter.Offset))

		rows, err := tx.Query(txCtx, selectQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var iv billing.AdminInvoiceView
			var statusStr string
			var issueDate, dueDate time.Time
			if err := rows.Scan(
				&iv.ID, &iv.PublicID, &iv.OrganizationID, &iv.VendorName,
				&iv.CustomerOrgID, &iv.CustomerName, &iv.OrderID, &iv.OrderNumber,
				&iv.InvoiceNumber, &issueDate, &dueDate, &iv.Subtotal,
				&iv.TaxAmount, &iv.DiscountAmount, &iv.TotalAmount, &statusStr,
				&iv.PaymentMethod, &iv.Notes, &iv.CreatedAt, &iv.PaidAmount,
			); err != nil {
				return err
			}
			iv.IssueDate = issueDate
			iv.DueDate = dueDate
			iv.Status = billing.InvoiceStatus(statusStr)
			if iv.TotalAmount.Minor() > iv.PaidAmount.Minor() {
				iv.RemainingAmount = money.FromMinor(iv.TotalAmount.Minor() - iv.PaidAmount.Minor())
			} else {
				iv.RemainingAmount = money.FromMinor(0)
			}
			if iv.PaidAmount.Minor() > 0 && iv.RemainingAmount.Minor() > 0 && iv.Status != billing.InvoiceCancelled {
				iv.Status = billing.InvoicePartiallyPaid
			}
			list = append(list, &iv)
		}
		return rows.Err()
	})
	return list, total, err
}
