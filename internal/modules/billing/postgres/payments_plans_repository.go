package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreatePayment inserts a payment record.
func (r *Repository) CreatePayment(ctx context.Context, p *billing.Payment) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO billing.payments (
				payment_integration_id, order_id, user_id, organization_id,
				amount, method, status, transaction_id, reference_number, paid_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			p.PaymentIntegrationID, p.OrderID, p.UserID, p.OrganizationID,
			p.Amount, p.Method, p.Status, p.TransactionID, p.ReferenceNumber, p.PaidAt,
		).Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
	})
}

// GetPaymentByID retrieves a payment by ID.
func (r *Repository) GetPaymentByID(ctx context.Context, id int64) (*billing.Payment, error) {
	var p billing.Payment
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, payment_integration_id, order_id, user_id, organization_id,
			       amount, method, status, transaction_id, reference_number, paid_at, created_at, updated_at
			FROM billing.payments
			WHERE id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&p.ID, &p.PublicID, &p.PaymentIntegrationID, &p.OrderID, &p.UserID, &p.OrganizationID,
			&p.Amount, &p.Method, &p.Status, &p.TransactionID, &p.ReferenceNumber, &p.PaidAt,
			&p.CreatedAt, &p.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("payment")
		}
		return nil, err
	}
	return &p, nil
}

// ListPaymentsByOrg retrieves payments associated with an organization.
func (r *Repository) ListPaymentsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Payment, error) {
	list, _, err := r.ListPaymentsByOrgWithTotal(ctx, orgID, limit, offset)
	return list, err
}

// ListPaymentsByOrgWithTotal retrieves paginated payments associated with an organization with total count.
func (r *Repository) ListPaymentsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Payment, int, error) {
	var list []*billing.Payment
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `SELECT count(*) FROM billing.payments WHERE organization_id = $1;`, orgID).Scan(&total); err != nil {
			return err
		}

		const query = `
			SELECT id, public_id, payment_integration_id, order_id, user_id, organization_id,
			       amount, method, status, transaction_id, reference_number, paid_at,
			       created_at, updated_at
			FROM billing.payments
			WHERE organization_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 25
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
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
	return list, total, err
}

// ListPlans lists all active subscription plans.
func (r *Repository) ListPlans(ctx context.Context) ([]*billing.Plan, error) {
	var plans []*billing.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, slug, name, description, price_month, price_year,
			       duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active, created_at, updated_at
			FROM billing.plans
			WHERE is_active = true
			ORDER BY id ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p billing.Plan
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Description,
				&p.PriceMonth, &p.PriceYear, &p.DurationDays, &p.MaxUsers,
				&p.MaxLoginSessions, &p.MaxDevices, &p.AIPlanID, &p.IsDefault,
				&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
			plans = append(plans, &p)
		}
		if rows.Err() != nil {
			return rows.Err()
		}

		for _, p := range plans {
			p.Features = loadPlanFeatures(txCtx, tx, p.ID)
		}
		return nil
	})
	return plans, err
}

// GetPlanByID retrieves a plan by ID.
func (r *Repository) GetPlanByID(ctx context.Context, id int64) (*billing.Plan, error) {
	var p billing.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, slug, name, description, price_month, price_year,
			       duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active, created_at, updated_at
			FROM billing.plans
			WHERE id = $1;
		`
		if err := tx.QueryRow(txCtx, query, id).Scan(
			&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Description,
			&p.PriceMonth, &p.PriceYear, &p.DurationDays, &p.MaxUsers,
			&p.MaxLoginSessions, &p.MaxDevices, &p.AIPlanID, &p.IsDefault,
			&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return err
		}
		p.Features = loadPlanFeatures(txCtx, tx, p.ID)
		return nil
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("plan")
		}
		return nil, err
	}
	return &p, nil
}
