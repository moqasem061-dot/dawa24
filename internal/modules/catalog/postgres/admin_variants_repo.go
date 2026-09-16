package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListAllVariants retrieves all product variants across all organizations (for platform admin) with pagination, search, and stock quantity aggregation.
func (r *Repository) ListAllVariants(ctx context.Context, params catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	var variants []*catalog.ProductVariant
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		whereClauses := []string{"v.deleted_at IS NULL"}
		args := []any{}
		argIdx := 1

		if params.Status != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("v.status = $%d", argIdx))
			args = append(args, params.Status)
			argIdx++
		}

		if params.Query != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("(v.name->>'ar' ILIKE $%d OR v.name->>'en' ILIKE $%d OR v.sku ILIKE $%d OR v.barcode ILIKE $%d OR v.batch_number ILIKE $%d)", argIdx, argIdx, argIdx, argIdx, argIdx))
			args = append(args, "%"+strings.TrimSpace(params.Query)+"%")
			argIdx++
		}

		whereSQL := strings.Join(whereClauses, " AND ")

		// 1. Count query
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM catalog.product_variants v WHERE %s;", whereSQL)
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return fmt.Errorf("count all variants: %w", err)
		}

		limit := params.Limit
		if limit <= 0 {
			limit = 50
		}
		offset := params.Offset
		if offset < 0 {
			offset = 0
		}

		variants = make([]*catalog.ProductVariant, 0, limit)

		// 2. Data query with aggregated stock quantity (ignoring soft-deleted stocks)
		dataQuery := fmt.Sprintf(`
			SELECT v.id, v.public_id, v.organization_id, v.product_id, v.name, v.sku, v.barcode,
			       v.price, v.cost_price, COALESCE(v.cost_discount_percentage, 0.00), v.discount, v.unit, v.image, v.status, v.is_featured, v.is_negotiable,
			       v.batch_number, v.expiry_date, v.min_order_qty, v.quota_limit, v.branch_id,
			       COALESCE(SUM(s.quantity), 0) as stock_qty,
			       v.created_at, v.updated_at, v.deleted_at
			FROM catalog.product_variants v
			LEFT JOIN inventory.stocks s ON s.product_variant_id = v.id AND s.deleted_at IS NULL
			WHERE %s
			GROUP BY v.id
			ORDER BY v.id DESC
			LIMIT $%d OFFSET $%d;
		`, whereSQL, argIdx, argIdx+1)

		args = append(args, limit, offset)
		rows, err := tx.Query(txCtx, dataQuery, args...)
		if err != nil {
			return fmt.Errorf("list all variants: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.ProductVariant
			var statusStr string
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
				&v.Barcode, &v.Price, &v.CostPrice, &v.CostDiscountPercentage, &v.Discount, &v.Unit, &v.Image,
				&statusStr, &v.IsFeatured, &v.IsNegotiable, &v.BatchNumber, &v.ExpiryDate, &v.MinOrderQty, &v.QuotaLimit,
				&v.BranchID, &v.StockQty, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
			); err != nil {
				return err
			}
			v.Status = catalog.ProductStatus(statusStr)
			variants = append(variants, &v)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, 0, err
	}
	return variants, total, nil
}

// UpdateVariant updates a variant.
//
// Scoped by organization as well as by id. The UI handler checks ownership
// before calling, which is right and is not enough: this method is exported and
// the WHERE clause is the only thing that makes it safe for the next caller.
func (r *Repository) UpdateVariant(ctx context.Context, v *catalog.ProductVariant) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.product_variants
			SET name = $2, sku = $3, barcode = $4, price = $5, cost_price = $6, cost_discount_percentage = $7,
			    discount = $8, unit = $9, image = $10, status = $11,
			    is_featured = $12, is_negotiable = $13, batch_number = $14, expiry_date = $15,
			    min_order_qty = $16, quota_limit = $17,
			    branch_id = COALESCE($18, branch_id, (SELECT b.id FROM org.branches b WHERE b.organization_id = catalog.product_variants.organization_id AND b.deleted_at IS NULL ORDER BY b.is_main DESC, b.id ASC LIMIT 1)),
			    updated_at = now()
			WHERE id = $1
			  AND deleted_at IS NULL
			  AND ($19::bigint = 0 OR organization_id = $19);
		`
		minQty := v.MinOrderQty
		if minQty <= 0 {
			minQty = 1
		}
		res, err := tx.Exec(txCtx, query,
			v.ID, v.Name, v.SKU, v.Barcode, v.Price, v.CostPrice, v.CostDiscountPercentage, v.Discount,
			v.Unit, v.Image, string(v.Status), v.IsFeatured, v.IsNegotiable, v.BatchNumber,
			v.ExpiryDate, minQty, catalog.NormalizeQuotaLimit(v.QuotaLimit), v.BranchID, v.OrganizationID,
		)
		if err != nil {
			// The partial unique index on (organization_id, sku) is the only
			// thing standing between two variants of one supplier sharing a
			// code, and a 23505 reaching the vendor as a 500 tells them
			// nothing about which field to change.
			if database.IsUniqueViolation(err) {
				return apperr.Conflict("variant.duplicate_sku",
					"That SKU already belongs to another item in this organization.")
			}
			return fmt.Errorf("catalog postgres: update variant: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("product_variant")
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &v.OrganizationID,
			Action:         "catalog.variant.update",
			EntityType:     "variant",
			EntityID:       strconv.FormatInt(v.ID, 10),
			After:          map[string]any{"sku": v.SKU, "price": v.Price, "status": v.Status},
		})

		return nil
	})
}

// DeleteVariant soft-deletes a product variant and cascades to warehouse stocks.
func (r *Repository) DeleteVariant(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.product_variants SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;`
		res, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return fmt.Errorf("catalog postgres: delete variant: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("product_variant")
		}

		// Cascade soft-delete to associated warehouse stocks
		_, _ = tx.Exec(txCtx, `UPDATE inventory.stocks SET deleted_at = now() WHERE product_variant_id = $1 AND deleted_at IS NULL;`, id)

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "catalog.variant.delete",
			EntityType: "variant",
			EntityID:   strconv.FormatInt(id, 10),
		})

		return nil
	})
}

// ToggleVariantStatus atomically switches a variant's status between active and inactive.
func (r *Repository) ToggleVariantStatus(ctx context.Context, orgID, variantID int64) (catalog.ProductStatus, error) {
	var newStatus string
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.product_variants
			SET status = CASE WHEN status = 'active' THEN 'inactive' ELSE 'active' END,
			    updated_at = now()
			WHERE id = $1 AND ($2::bigint = 0 OR organization_id = $2) AND deleted_at IS NULL
			RETURNING status;
		`
		if err := tx.QueryRow(txCtx, query, variantID, orgID).Scan(&newStatus); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("product_variant")
			}
			return fmt.Errorf("catalog postgres: toggle variant status: %w", err)
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &orgID,
			Action:         "catalog.variant.toggle_status",
			EntityType:     "variant",
			EntityID:       strconv.FormatInt(variantID, 10),
			After:          map[string]any{"status": newStatus},
		})
		return nil
	})
	return catalog.ProductStatus(newStatus), err
}
