package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// SetCustomerPricing upserts per-customer custom pricing (071: canonical
// price/discount columns).
func (r *Repository) SetCustomerPricing(ctx context.Context, m *catalog.CustomerProductMapping) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.customer_product_mappings (
				organization_id, customer_org_id, product_id, product_variant_id, price, discount, is_active
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (organization_id, customer_org_id, product_id, product_variant_id)
			DO UPDATE SET price = EXCLUDED.price, discount = EXCLUDED.discount, is_active = EXCLUDED.is_active, updated_at = now()
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			m.OrganizationID, m.CustomerOrgID, m.ProductID, m.ProductVariantID, m.Price, m.Discount, m.IsActive,
		).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	})
}

// GetCustomerPricing looks up customer-specific custom pricing.
func (r *Repository) GetCustomerPricing(ctx context.Context, vendorOrgID, customerOrgID, productID int64) (*catalog.CustomerProductMapping, error) {
	var m catalog.CustomerProductMapping
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, organization_id, customer_org_id, product_id, product_variant_id, price, discount, is_active, created_at, updated_at
			FROM catalog.customer_product_mappings
			WHERE organization_id = $1 AND customer_org_id = $2 AND product_id = $3 AND is_active = true
			LIMIT 1;
		`
		err := tx.QueryRow(txCtx, query, vendorOrgID, customerOrgID, productID).Scan(
			&m.ID, &m.OrganizationID, &m.CustomerOrgID, &m.ProductID, &m.ProductVariantID,
			&m.Price, &m.Discount, &m.IsActive, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("customer_product_mapping")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// CreateProductAlert registers a price/stock alert.
func (r *Repository) CreateProductAlert(ctx context.Context, a *catalog.ProductAlert) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.product_alerts (user_id, product_id, alert_type, target_price)
			VALUES ($1, $2, $3, $4)
			RETURNING id, public_id, created_at;
		`
		return tx.QueryRow(txCtx, query, a.UserID, a.ProductID, a.AlertType, a.TargetPrice).
			Scan(&a.ID, &a.PublicID, &a.CreatedAt)
	})
}

// ListProductAlertsByUser lists active alerts for a user.
func (r *Repository) ListProductAlertsByUser(ctx context.Context, userID int64) ([]*catalog.ProductAlert, error) {
	var list []*catalog.ProductAlert
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, product_id, alert_type, target_price, is_triggered, triggered_at, created_at
			FROM catalog.product_alerts
			WHERE user_id = $1 AND is_triggered = false
			ORDER BY created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a catalog.ProductAlert
			if err := rows.Scan(&a.ID, &a.PublicID, &a.UserID, &a.ProductID, &a.AlertType, &a.TargetPrice, &a.IsTriggered, &a.TriggeredAt, &a.CreatedAt); err != nil {
				return err
			}
			list = append(list, &a)
		}
		return rows.Err()
	})
	return list, err
}
