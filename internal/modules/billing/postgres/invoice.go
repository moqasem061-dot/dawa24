package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CreateInvoice persists a B2B invoice and its line items.
func (r *Repository) CreateInvoice(ctx context.Context, inv *billing.Invoice) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO billing.invoices (
				organization_id, customer_org_id, order_id, invoice_number,
				issue_date, due_date, subtotal, tax_amount, discount_amount,
				total_amount, status, payment_method, notes
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			RETURNING id, public_id, created_at, updated_at;
		`
		if inv.IssueDate.IsZero() {
			inv.IssueDate = time.Now().UTC()
		}
		if inv.DueDate.IsZero() {
			inv.DueDate = inv.IssueDate.AddDate(0, 0, 30)
		}

		err := tx.QueryRow(txCtx, query,
			inv.OrganizationID, inv.CustomerOrgID, inv.OrderID, inv.InvoiceNumber,
			inv.IssueDate, inv.DueDate, inv.Subtotal, inv.TaxAmount, inv.DiscountAmount,
			inv.TotalAmount, string(inv.Status), inv.PaymentMethod, inv.Notes,
		).Scan(&inv.ID, &inv.PublicID, &inv.CreatedAt, &inv.UpdatedAt)
		if err != nil {
			return err
		}

		for i := range inv.Lines {
			line := &inv.Lines[i]
			lineQuery := `
				INSERT INTO billing.invoice_lines (invoice_id, product_id, description, quantity, unit_price, total_price)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id;
			`
			if err := tx.QueryRow(txCtx, lineQuery,
				inv.ID, line.ProductID, line.Description, line.Quantity, line.UnitPrice, line.TotalPrice,
			).Scan(&line.ID); err != nil {
				return err
			}
			line.InvoiceID = inv.ID
		}
		return nil
	})
}

// GetInvoiceByID retrieves an invoice and its lines.
func (r *Repository) GetInvoiceByID(ctx context.Context, id int64) (*billing.Invoice, error) {
	var inv billing.Invoice
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT inv.id, inv.public_id, inv.organization_id, inv.customer_org_id, inv.order_id, inv.invoice_number,
			       inv.issue_date, inv.due_date, inv.subtotal, inv.tax_amount, inv.discount_amount, inv.total_amount,
			       inv.status, inv.payment_method, inv.notes, inv.created_at, inv.updated_at,
			       COALESCE((SELECT SUM(p.amount) FROM billing.payments p WHERE (p.invoice_id = inv.id OR (p.invoice_id IS NULL AND p.order_id IS NOT NULL AND p.order_id = inv.order_id)) AND p.status IN ('paid', 'completed')), 0) AS paid_amount
			FROM billing.invoices inv WHERE inv.id = $1;
		`
		var statusStr string
		var notes *string
		if err := tx.QueryRow(txCtx, query, id).Scan(
			&inv.ID, &inv.PublicID, &inv.OrganizationID, &inv.CustomerOrgID, &inv.OrderID, &inv.InvoiceNumber,
			&inv.IssueDate, &inv.DueDate, &inv.Subtotal, &inv.TaxAmount, &inv.DiscountAmount, &inv.TotalAmount,
			&statusStr, &inv.PaymentMethod, &notes, &inv.CreatedAt, &inv.UpdatedAt, &inv.PaidAmount,
		); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("invoice")
			}
			return err
		}
		inv.Status = billing.InvoiceStatus(statusStr)
		if notes != nil {
			inv.Notes = *notes
		}
		if inv.TotalAmount.Minor() > inv.PaidAmount.Minor() {
			inv.RemainingAmount = money.FromMinor(inv.TotalAmount.Minor() - inv.PaidAmount.Minor())
		} else {
			inv.RemainingAmount = money.FromMinor(0)
		}
		if inv.PaidAmount.Minor() > 0 && inv.RemainingAmount.Minor() > 0 && inv.Status != billing.InvoiceCancelled {
			inv.Status = billing.InvoicePartiallyPaid
		}

		linesQuery := `SELECT id, invoice_id, product_id, description, quantity, unit_price, total_price FROM billing.invoice_lines WHERE invoice_id = $1;`
		rows, err := tx.Query(txCtx, linesQuery, id)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var line billing.InvoiceLine
			if err := rows.Scan(&line.ID, &line.InvoiceID, &line.ProductID, &line.Description, &line.Quantity, &line.UnitPrice, &line.TotalPrice); err != nil {
				return err
			}
			inv.Lines = append(inv.Lines, line)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// GetInvoiceByOrderID retrieves an invoice and its lines by associated order ID.
func (r *Repository) GetInvoiceByOrderID(ctx context.Context, orderID int64) (*billing.Invoice, error) {
	var inv billing.Invoice
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT inv.id, inv.public_id, inv.organization_id, inv.customer_org_id, inv.order_id, inv.invoice_number,
			       inv.issue_date, inv.due_date, inv.subtotal, inv.tax_amount, inv.discount_amount, inv.total_amount,
			       inv.status, inv.payment_method, inv.notes, inv.created_at, inv.updated_at,
			       COALESCE((SELECT SUM(p.amount) FROM billing.payments p WHERE (p.invoice_id = inv.id OR (p.invoice_id IS NULL AND p.order_id IS NOT NULL AND p.order_id = inv.order_id)) AND p.status IN ('paid', 'completed')), 0) AS paid_amount
			FROM billing.invoices inv WHERE inv.order_id = $1 ORDER BY inv.id DESC LIMIT 1;
		`
		var statusStr string
		var notes *string
		if err := tx.QueryRow(txCtx, query, orderID).Scan(
			&inv.ID, &inv.PublicID, &inv.OrganizationID, &inv.CustomerOrgID, &inv.OrderID, &inv.InvoiceNumber,
			&inv.IssueDate, &inv.DueDate, &inv.Subtotal, &inv.TaxAmount, &inv.DiscountAmount, &inv.TotalAmount,
			&statusStr, &inv.PaymentMethod, &notes, &inv.CreatedAt, &inv.UpdatedAt, &inv.PaidAmount,
		); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("invoice")
			}
			return err
		}
		inv.Status = billing.InvoiceStatus(statusStr)
		if notes != nil {
			inv.Notes = *notes
		}
		if inv.TotalAmount.Minor() > inv.PaidAmount.Minor() {
			inv.RemainingAmount = money.FromMinor(inv.TotalAmount.Minor() - inv.PaidAmount.Minor())
		} else {
			inv.RemainingAmount = money.FromMinor(0)
		}
		if inv.PaidAmount.Minor() > 0 && inv.RemainingAmount.Minor() > 0 && inv.Status != billing.InvoiceCancelled {
			inv.Status = billing.InvoicePartiallyPaid
		}

		linesQuery := `SELECT id, invoice_id, product_id, description, quantity, unit_price, total_price FROM billing.invoice_lines WHERE invoice_id = $1;`
		rows, err := tx.Query(txCtx, linesQuery, inv.ID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var line billing.InvoiceLine
			if err := rows.Scan(&line.ID, &line.InvoiceID, &line.ProductID, &line.Description, &line.Quantity, &line.UnitPrice, &line.TotalPrice); err != nil {
				return err
			}
			inv.Lines = append(inv.Lines, line)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// UpdateInvoiceStatus changes invoice status.
func (r *Repository) UpdateInvoiceStatus(ctx context.Context, id int64, status billing.InvoiceStatus) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE billing.invoices SET status = $1, updated_at = now() WHERE id = $2;`
		tag, err := tx.Exec(txCtx, query, string(status), id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("invoice")
		}
		return nil
	})
}

// ListInvoicesByOrg returns invoices for an organization.
func (r *Repository) ListInvoicesByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Invoice, error) {
	list, _, err := r.ListInvoicesByOrgWithTotal(ctx, orgID, limit, offset)
	return list, err
}

// ListInvoicesByOrgWithTotal returns paginated invoices for an organization with total count.
func (r *Repository) ListInvoicesByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Invoice, int, error) {
	var list []*billing.Invoice
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `
			SELECT count(*)
			FROM billing.invoices
			WHERE organization_id = $1 OR customer_org_id = $1;
		`
		if err := tx.QueryRow(txCtx, countQuery, orgID).Scan(&total); err != nil {
			return err
		}

		query := `
			SELECT id, public_id, organization_id, customer_org_id, order_id, invoice_number,
			       issue_date, due_date, subtotal, tax_amount, discount_amount, total_amount,
			       status, payment_method, notes, created_at, updated_at
			FROM billing.invoices
			WHERE organization_id = $1 OR customer_org_id = $1
			ORDER BY issue_date DESC, id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var inv billing.Invoice
			var statusStr string
			var notes *string
			if err := rows.Scan(
				&inv.ID, &inv.PublicID, &inv.OrganizationID, &inv.CustomerOrgID, &inv.OrderID, &inv.InvoiceNumber,
				&inv.IssueDate, &inv.DueDate, &inv.Subtotal, &inv.TaxAmount, &inv.DiscountAmount, &inv.TotalAmount,
				&statusStr, &inv.PaymentMethod, &notes, &inv.CreatedAt, &inv.UpdatedAt,
			); err != nil {
				return err
			}
			inv.Status = billing.InvoiceStatus(statusStr)
			if notes != nil {
				inv.Notes = *notes
			}
			list = append(list, &inv)
		}
		return rows.Err()
	})
	return list, total, err
}

// AddPaymentMethod stores a user payment method.
func (r *Repository) AddPaymentMethod(ctx context.Context, pm *billing.UserPaymentMethod) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if pm.IsDefault {
			if _, err := tx.Exec(txCtx, `UPDATE billing.user_payment_methods SET is_default = false WHERE user_id = $1;`, pm.UserID); err != nil {
				return err
			}
		}
		query := `
			INSERT INTO billing.user_payment_methods (user_id, provider, account_identifier, details, is_default)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, public_id, created_at;
		`
		return tx.QueryRow(txCtx, query, pm.UserID, pm.Provider, pm.AccountIdentifier, pm.Details, pm.IsDefault).
			Scan(&pm.ID, &pm.PublicID, &pm.CreatedAt)
	})
}

// GetPaymentMethodByID returns a single payment method for a user.
func (r *Repository) GetPaymentMethodByID(ctx context.Context, userID, id int64) (*billing.UserPaymentMethod, error) {
	var pm billing.UserPaymentMethod
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, user_id, provider, account_identifier, COALESCE(details, '{}'::jsonb), is_default, created_at FROM billing.user_payment_methods WHERE id = $1 AND user_id = $2;`
		return tx.QueryRow(txCtx, query, id, userID).
			Scan(&pm.ID, &pm.PublicID, &pm.UserID, &pm.Provider, &pm.AccountIdentifier, &pm.Details, &pm.IsDefault, &pm.CreatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("payment_method")
		}
		return nil, err
	}
	return &pm, nil
}

// ListPaymentMethods returns saved payment methods for a user.
func (r *Repository) ListPaymentMethods(ctx context.Context, userID int64) ([]*billing.UserPaymentMethod, error) {
	var list []*billing.UserPaymentMethod
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, user_id, provider, account_identifier, COALESCE(details, '{}'::jsonb), is_default, created_at FROM billing.user_payment_methods WHERE user_id = $1 ORDER BY is_default DESC, id DESC;`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pm billing.UserPaymentMethod
			if err := rows.Scan(&pm.ID, &pm.PublicID, &pm.UserID, &pm.Provider, &pm.AccountIdentifier, &pm.Details, &pm.IsDefault, &pm.CreatedAt); err != nil {
				return err
			}
			list = append(list, &pm)
		}
		return rows.Err()
	})
	return list, err
}

// UpdatePaymentMethod updates an existing payment method for a user.
func (r *Repository) UpdatePaymentMethod(ctx context.Context, pm *billing.UserPaymentMethod) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if pm.IsDefault {
			if _, err := tx.Exec(txCtx, `UPDATE billing.user_payment_methods SET is_default = false WHERE user_id = $1;`, pm.UserID); err != nil {
				return err
			}
		}
		query := `
			UPDATE billing.user_payment_methods
			SET provider = $1, account_identifier = $2, details = $3, is_default = $4
			WHERE id = $5 AND user_id = $6;
		`
		tag, err := tx.Exec(txCtx, query, pm.Provider, pm.AccountIdentifier, pm.Details, pm.IsDefault, pm.ID, pm.UserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("payment_method")
		}
		return nil
	})
}

// SetDefaultPaymentMethod marks a specific payment method as default and clears others.
func (r *Repository) SetDefaultPaymentMethod(ctx context.Context, userID, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `UPDATE billing.user_payment_methods SET is_default = false WHERE user_id = $1;`, userID); err != nil {
			return err
		}
		tag, err := tx.Exec(txCtx, `UPDATE billing.user_payment_methods SET is_default = true WHERE id = $1 AND user_id = $2;`, id, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("payment_method")
		}
		return nil
	})
}

// DeletePaymentMethod removes a payment method belonging to the given user.
func (r *Repository) DeletePaymentMethod(ctx context.Context, userID, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM billing.user_payment_methods WHERE id = $1 AND user_id = $2;`
		tag, err := tx.Exec(txCtx, query, id, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("payment_method")
		}
		return nil
	})
}
