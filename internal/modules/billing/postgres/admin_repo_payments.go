package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// GetVendorPaymentStats returns KPI aggregation for vendor payments.
func (r *Repository) GetVendorPaymentStats(ctx context.Context, orgID int64) (*billing.VendorPaymentStats, error) {
	stats := &billing.VendorPaymentStats{}
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			SELECT 
				COUNT(*),
				COALESCE(SUM(p.amount), 0)::text,
				COALESCE(SUM(CASE WHEN (p.paid_at >= CURRENT_DATE OR p.created_at >= CURRENT_DATE) THEN p.amount ELSE 0 END), 0)::text,
				COALESCE(SUM(CASE WHEN (p.paid_at >= date_trunc('month', CURRENT_DATE) OR p.created_at >= date_trunc('month', CURRENT_DATE)) THEN p.amount ELSE 0 END), 0)::text
			FROM billing.payments p
			LEFT JOIN billing.invoices inv ON p.invoice_id = inv.id
			WHERE (p.organization_id = $1 OR inv.organization_id = $1) AND p.status IN ('paid', 'completed');
		`, orgID)
		return row.Scan(&stats.TotalCount, &stats.TotalAmount, &stats.TodayAmount, &stats.MonthAmount)
	})
	return stats, err
}

// RecordInvoicePayment records a payment against an invoice, updates invoice status (including partially_paid), and keeps order synced.
func (r *Repository) RecordInvoicePayment(ctx context.Context, req billing.RecordInvoicePaymentRequest) (*billing.Payment, error) {
	if req.InvoiceID <= 0 {
		return nil, apperr.Validation("payment.invoice_required", "يجب تحديد الفاتورة المرتبطة بالدفعة", nil)
	}
	if req.Amount.Minor() <= 0 {
		return nil, apperr.Validation("payment.amount_positive", "يجب أن يكون مبلغ الدفعة أكبر من صفر", nil)
	}

	var p billing.Payment
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Fetch invoice
		var invID, orgID int64
		var custOrgID *int64
		var orderID *int64
		var invTotal money.Amount
		var invStatus string
		err := tx.QueryRow(txCtx, `
			SELECT id, organization_id, customer_org_id, order_id, total_amount, status
			FROM billing.invoices
			WHERE id = $1;
		`, req.InvoiceID).Scan(&invID, &orgID, &custOrgID, &orderID, &invTotal, &invStatus)
		if err != nil {
			return fmt.Errorf("invoice not found: %w", err)
		}

		if req.OrganizationID > 0 && req.OrganizationID != orgID && (custOrgID == nil || req.OrganizationID != *custOrgID) {
			return apperr.Forbidden("payment.forbidden", "غير مصرح لك بتسجيل دفعة على هذه الفاتورة")
		}

		// 2. Generate transaction ID if empty
		txID := req.ReferenceNumber
		if txID == "" {
			txID = fmt.Sprintf("TXN-%s", strings.ToUpper(uuid.NewString()[:8]))
		}

		refNum := req.ReferenceNumber
		if refNum == "" {
			refNum = txID
		}

		method := strings.TrimSpace(req.Method)
		if method == "" {
			method = "bank_transfer"
		}

		paidAt := req.PaidAt
		if paidAt == nil {
			now := time.Now()
			paidAt = &now
		}

		// The payment is attributed to a person in the paying organisation. The
		// table is org.members — org.memberships has never existed, so this
		// lookup always failed, silently, and every payment recorded without an
		// explicit user fell through to "the lowest user id in the system" and
		// was filed against a stranger.
		userID := req.UserID
		if userID <= 0 {
			if err := tx.QueryRow(txCtx, `
				SELECT user_id
				FROM org.members
				WHERE organization_id = $1 AND status = 'active'
				ORDER BY joined_at ASC, id ASC
				LIMIT 1
			`, orgID).Scan(&userID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("resolve paying member for organization %d: %w", orgID, err)
			}
		}
		if userID <= 0 {
			// An organisation with no active member cannot own a payment, and
			// guessing a user here is how the wrong person ends up on the
			// receipt. Refuse instead.
			return apperr.Validation("payment.no_member",
				"the organization has no active member to attribute this payment to", nil)
		}

		// 3. Insert payment (using 'paid' to comply with payments_status_check constraint)
		const insertPayment = `
			INSERT INTO billing.payments (
				invoice_id, order_id, user_id, organization_id, amount,
				method, status, transaction_id, reference_number, notes, paid_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, 'paid', $7, $8, $9, $10
			) RETURNING id, public_id::text, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, insertPayment,
			invID, orderID, userID, orgID, req.Amount,
			method, txID, refNum, req.Notes, paidAt,
		).Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return fmt.Errorf("insert payment: %w", err)
		}

		p.InvoiceID = &invID
		p.OrderID = orderID
		p.UserID = userID
		p.OrganizationID = &orgID
		p.Amount = req.Amount
		p.Method = method
		p.Status = "paid"
		p.TransactionID = txID
		p.ReferenceNumber = refNum
		p.PaidAt = paidAt

		// 4. Calculate total paid on this invoice
		var totalPaid money.Amount
		err = tx.QueryRow(txCtx, `
			SELECT COALESCE(SUM(amount), 0)::text
			FROM billing.payments
			WHERE (invoice_id = $1 OR (invoice_id IS NULL AND order_id IS NOT NULL AND order_id = $2))
			  AND status IN ('paid', 'completed');
		`, invID, orderID).Scan(&totalPaid)
		if err != nil {
			return fmt.Errorf("calculate total paid: %w", err)
		}

		// 5. Update invoice status
		newStatus := "partially_paid"
		if totalPaid.Minor() >= invTotal.Minor() {
			newStatus = "paid"
		} else if totalPaid.Minor() <= 0 {
			newStatus = "issued"
		}

		_, err = tx.Exec(txCtx, `
			UPDATE billing.invoices
			SET status = $1, updated_at = now()
			WHERE id = $2;
		`, newStatus, invID)
		if err != nil {
			return fmt.Errorf("update invoice status: %w", err)
		}

		// 6. If linked to an order, keep order status updated
		if orderID != nil {
			orderPaymentStatus := "partially_paid"
			if newStatus == "paid" {
				orderPaymentStatus = "paid"
			}
			_, _ = tx.Exec(txCtx, `
				UPDATE commerce.orders
				SET payment_status = $1, updated_at = now()
				WHERE id = $2;
			`, orderPaymentStatus, *orderID)
		}

		return nil
	})

	return &p, err
}
