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

// CreateVariant inserts a product variant.
func (r *Repository) CreateVariant(ctx context.Context, v *catalog.ProductVariant) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.product_variants (
				organization_id, product_id, name, sku, barcode, price, cost_price, cost_discount_percentage,
				discount, unit, image, status, is_featured, is_negotiable, batch_number, expiry_date,
				min_order_qty, quota_limit, branch_id
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
				COALESCE($19, (SELECT b.id FROM org.branches b WHERE b.organization_id = $1 AND b.deleted_at IS NULL ORDER BY b.is_main DESC, b.id ASC LIMIT 1))
			) RETURNING id, public_id, created_at, updated_at;
		`
		minQty := v.MinOrderQty
		if minQty <= 0 {
			minQty = 1
		}
		err := tx.QueryRow(txCtx, query,
			v.OrganizationID, v.ProductID, v.Name, v.SKU, v.Barcode, v.Price,
			v.CostPrice, v.CostDiscountPercentage, v.Discount, v.Unit, v.Image, string(v.Status), v.IsFeatured, v.IsNegotiable,
			v.BatchNumber, v.ExpiryDate, minQty, catalog.NormalizeQuotaLimit(v.QuotaLimit), v.BranchID,
		).Scan(&v.ID, &v.PublicID, &v.CreatedAt, &v.UpdatedAt)

		if err != nil {
			return fmt.Errorf("catalog postgres: create variant: %w", err)
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &v.OrganizationID,
			Action:         "catalog.variant.create",
			EntityType:     "variant",
			EntityID:       strconv.FormatInt(v.ID, 10),
			After:          map[string]any{"sku": v.SKU, "price": v.Price, "status": v.Status},
		})
		return nil
	})
}

// GetVariantByID retrieves a variant by ID.
func (r *Repository) GetVariantByID(ctx context.Context, id int64) (*catalog.ProductVariant, error) {
	var v catalog.ProductVariant
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, product_id, name, sku, barcode,
			       price, cost_price, COALESCE(cost_discount_percentage, 0.00), discount, unit, image, status, is_featured, is_negotiable,
			       batch_number, expiry_date, min_order_qty, quota_limit, branch_id,
			       created_at, updated_at, deleted_at
			FROM catalog.product_variants
			WHERE id = $1 AND deleted_at IS NULL;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
			&v.Barcode, &v.Price, &v.CostPrice, &v.CostDiscountPercentage, &v.Discount, &v.Unit, &v.Image,
			&statusStr, &v.IsFeatured, &v.IsNegotiable, &v.BatchNumber, &v.ExpiryDate, &v.MinOrderQty, &v.QuotaLimit,
			&v.BranchID, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("product_variant")
			}
			return fmt.Errorf("catalog postgres: get variant: %w", err)
		}
		v.Status = catalog.ProductStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetVariantBySKUOrBarcode retrieves a variant by SKU or Barcode within an organization.
func (r *Repository) GetVariantBySKUOrBarcode(ctx context.Context, orgID int64, sku, barcode string) (*catalog.ProductVariant, error) {
	if sku == "" && barcode == "" {
		return nil, apperr.NotFound("product_variant")
	}
	var v catalog.ProductVariant
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, product_id, name, sku, barcode,
			       price, cost_price, COALESCE(cost_discount_percentage, 0.00), discount, unit, image, status, is_featured, is_negotiable,
			       batch_number, expiry_date, min_order_qty, quota_limit, branch_id,
			       created_at, updated_at, deleted_at
			FROM catalog.product_variants
			WHERE organization_id = $1 AND deleted_at IS NULL
			  AND (($2 <> '' AND sku = $2) OR ($3 <> '' AND barcode = $3))
			LIMIT 1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, orgID, sku, barcode).Scan(
			&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
			&v.Barcode, &v.Price, &v.CostPrice, &v.CostDiscountPercentage, &v.Discount, &v.Unit, &v.Image,
			&statusStr, &v.IsFeatured, &v.IsNegotiable, &v.BatchNumber, &v.ExpiryDate, &v.MinOrderQty, &v.QuotaLimit,
			&v.BranchID, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("product_variant")
			}
			return fmt.Errorf("catalog postgres: get variant by sku/barcode: %w", err)
		}
		v.Status = catalog.ProductStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetVariantByProductAndOrg retrieves a variant by parent product ID and organization ID.
func (r *Repository) GetVariantByProductAndOrg(ctx context.Context, orgID int64, productID int64) (*catalog.ProductVariant, error) {
	var v catalog.ProductVariant
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, product_id, name, sku, barcode,
			       price, cost_price, COALESCE(cost_discount_percentage, 0.00), discount, unit, image, status, is_featured, is_negotiable,
			       batch_number, expiry_date, min_order_qty, quota_limit, branch_id,
			       created_at, updated_at, deleted_at
			FROM catalog.product_variants
			WHERE organization_id = $1 AND product_id = $2 AND deleted_at IS NULL
			LIMIT 1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, orgID, productID).Scan(
			&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
			&v.Barcode, &v.Price, &v.CostPrice, &v.CostDiscountPercentage, &v.Discount, &v.Unit, &v.Image,
			&statusStr, &v.IsFeatured, &v.IsNegotiable, &v.BatchNumber, &v.ExpiryDate, &v.MinOrderQty, &v.QuotaLimit,
			&v.BranchID, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("product_variant")
			}
			return fmt.Errorf("catalog postgres: get variant by prod/org: %w", err)
		}
		v.Status = catalog.ProductStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ListVariantsByProduct retrieves all active variants of a product.
func (r *Repository) ListVariantsByProduct(ctx context.Context, productID int64) ([]*catalog.ProductVariant, error) {
	var variants []*catalog.ProductVariant
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, product_id, name, sku, barcode,
			       price, cost_price, COALESCE(cost_discount_percentage, 0.00), discount, unit, image, status, is_featured, is_negotiable,
			       batch_number, expiry_date, min_order_qty, quota_limit, branch_id,
			       created_at, updated_at, deleted_at
			FROM catalog.product_variants
			WHERE product_id = $1 AND deleted_at IS NULL
			ORDER BY id ASC;
		`
		rows, err := tx.Query(txCtx, query, productID)
		if err != nil {
			return fmt.Errorf("catalog postgres: list variants: %w", err)
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

// ListVariantsByOrganization retrieves variants belonging to an organization with pagination and search.
func (r *Repository) ListVariantsByOrganization(ctx context.Context, orgID int64, params catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	var variants []*catalog.ProductVariant
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		whereClauses := []string{"v.organization_id = $1", "v.deleted_at IS NULL"}
		args := []any{orgID}
		argIdx := 2

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
			return fmt.Errorf("count variants by org: %w", err)
		}

		limit := params.Limit
		if limit <= 0 {
			limit = 24
		}
		offset := params.Offset
		if offset < 0 {
			offset = 0
		}

		// 2. Data query
		dataQuery := fmt.Sprintf(`
			SELECT v.id, v.public_id, v.organization_id, v.product_id, v.name, v.sku, v.barcode,
			       v.price, v.cost_price, COALESCE(v.cost_discount_percentage, 0.00), v.discount, v.unit, v.image, v.status, v.is_featured, v.is_negotiable,
			       v.batch_number, v.expiry_date, v.min_order_qty, v.quota_limit, v.branch_id,
			       v.created_at, v.updated_at, v.deleted_at
			FROM catalog.product_variants v
			WHERE %s
			ORDER BY v.id DESC
			LIMIT $%d OFFSET $%d;
		`, whereSQL, argIdx, argIdx+1)

		args = append(args, limit, offset)
		rows, err := tx.Query(txCtx, dataQuery, args...)
		if err != nil {
			return fmt.Errorf("list variants by org: %w", err)
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
		return nil, 0, err
	}
	return variants, total, nil
}
