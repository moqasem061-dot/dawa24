package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListVariantsByProducts retrieves the active variants of many products in a
// single query, keyed by parent product ID by the caller. The storefront
// catalog previously issued one query per product per page view.
func (r *Repository) ListVariantsByProducts(ctx context.Context, productIDs []int64) ([]*catalog.ProductVariant, error) {
	if len(productIDs) == 0 {
		return nil, nil
	}
	variants := make([]*catalog.ProductVariant, 0, len(productIDs))
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, product_id, name, sku, barcode,
			       price, cost_price, COALESCE(cost_discount_percentage, 0.00), discount, unit, image, status, is_featured, is_negotiable,
			       batch_number, expiry_date, min_order_qty, quota_limit, branch_id,
			       created_at, updated_at, deleted_at
			FROM catalog.product_variants
			WHERE product_id = ANY($1) AND deleted_at IS NULL
			ORDER BY id ASC;
		`
		rows, err := tx.Query(txCtx, query, productIDs)
		if err != nil {
			return fmt.Errorf("catalog postgres: list variants by products: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.ProductVariant
			var statusStr string
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
				&v.Barcode, &v.Price, &v.CostPrice, &v.CostDiscountPercentage, &v.Discount, &v.Unit, &v.Image,
				&statusStr, &v.IsFeatured, &v.IsNegotiable, &v.BatchNumber, &v.ExpiryDate, &v.MinOrderQty, &v.QuotaLimit,
				&v.BranchID, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
			); err != nil {
				return err
			}
			v.Status = catalog.ProductStatus(statusStr)
			variants = append(variants, &v)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return variants, nil
}

// GetVariantsByIDs retrieves many variants in one query, keyed by id.
//
// The landing page enriches every sponsored ad with its variant's pricing. One
// GetVariantByID per ad meant a query per ad, in sequence, before the page could
// render — the single biggest reason the home page took seconds to answer.
func (r *Repository) GetVariantsByIDs(ctx context.Context, ids []int64) (map[int64]*catalog.ProductVariant, error) {
	out := make(map[int64]*catalog.ProductVariant, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT id, public_id, organization_id, product_id, name, sku, barcode,
			       price, cost_price, COALESCE(cost_discount_percentage, 0.00), discount, unit, image, status, is_featured, is_negotiable,
			       batch_number, expiry_date, min_order_qty, quota_limit, branch_id,
			       created_at, updated_at, deleted_at
			FROM catalog.product_variants
			WHERE id = ANY($1) AND deleted_at IS NULL;`, ids)
		if err != nil {
			return fmt.Errorf("catalog postgres: get variants by ids: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.ProductVariant
			var statusStr string
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
				&v.Barcode, &v.Price, &v.CostPrice, &v.CostDiscountPercentage, &v.Discount, &v.Unit, &v.Image,
				&statusStr, &v.IsFeatured, &v.IsNegotiable, &v.BatchNumber, &v.ExpiryDate, &v.MinOrderQty, &v.QuotaLimit,
				&v.BranchID, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
			); err != nil {
				return err
			}
			v.Status = catalog.ProductStatus(statusStr)
			cp := v
			out[v.ID] = &cp
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteAllVariantsByOrg soft-deletes all variants belonging to an organization and clears their stocks.
func (r *Repository) DeleteAllVariantsByOrg(ctx context.Context, orgID int64) (int64, error) {
	var count int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `
			UPDATE catalog.product_variants
			SET deleted_at = now()
			WHERE organization_id = $1 AND deleted_at IS NULL;
		`, orgID)
		if err != nil {
			return fmt.Errorf("catalog postgres: delete all variants by org: %w", err)
		}
		count = res.RowsAffected()

		// Cascade soft-delete to associated warehouse stocks
		_, _ = tx.Exec(txCtx, `
			UPDATE inventory.stocks
			SET deleted_at = now()
			WHERE organization_id = $1 AND deleted_at IS NULL;
		`, orgID)

		return nil
	})
	return count, err
}

// ActivateAllVariantsByOrg sets all non-deleted variants of an organization to 'active'.
func (r *Repository) ActivateAllVariantsByOrg(ctx context.Context, orgID int64) (int64, error) {
	var count int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `
			UPDATE catalog.product_variants
			SET status = 'active', updated_at = now()
			WHERE organization_id = $1 AND status != 'active' AND deleted_at IS NULL;
		`, orgID)
		if err != nil {
			return fmt.Errorf("catalog postgres: activate all variants by org: %w", err)
		}
		count = res.RowsAffected()

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &orgID,
			Action:         "catalog.bulk.activate_all",
			EntityType:     "organization",
			EntityID:       fmt.Sprintf("%d", orgID),
			After:          map[string]any{"count": count},
		})

		return nil
	})
	return count, err
}

// DeleteAllProducts soft-deletes all master catalog products, variants, and warehouse stocks.
func (r *Repository) DeleteAllProducts(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `UPDATE catalog.product_variants SET deleted_at = now() WHERE deleted_at IS NULL;`)
		_, _ = tx.Exec(txCtx, `UPDATE inventory.stocks SET deleted_at = now() WHERE deleted_at IS NULL;`)
		res, err := tx.Exec(txCtx, `UPDATE catalog.products SET deleted_at = now() WHERE deleted_at IS NULL;`)
		if err != nil {
			return fmt.Errorf("catalog postgres: delete all master products: %w", err)
		}
		count = res.RowsAffected()

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "catalog.bulk.delete_all",
			EntityType: "catalog",
			EntityID:   "master",
			After:      map[string]any{"count": count},
		})

		return nil
	})
	return count, err
}
