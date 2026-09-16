package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Taxonomy deletion, vendor product listing and bulk status changes.

// ListProducts returns the active organization's own catalogue.
func (r *Repository) ListProducts(ctx context.Context, status string, limit, offset int) ([]*catalog.Product, error) {
	var list []*catalog.Product
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, category_id, brand_id, branch_id,
			       name, description, sku, barcode, price, discount, old_price,
			       image, image_link, status, is_featured, dosage_form,
			       scientific_name, pharmacology, active, concentration, unit,
			       manufacturing_companies, created_at, updated_at, deleted_at
			FROM catalog.products
			WHERE deleted_at IS NULL AND ($1 = '' OR status = $1)
			ORDER BY updated_at DESC, id DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p catalog.Product
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.OrganizationID, &p.CategoryID, &p.BrandID, &p.BranchID,
				&p.Name, &p.Description, &p.SKU, &p.Barcode, &p.Price, &p.Discount, &p.OldPrice,
				&p.Image, &p.ImageLink, &p.Status, &p.IsFeatured, &p.DosageForm,
				&p.ScientificName, &p.Pharmacology, &p.Active, &p.Concentration, &p.Unit,
				&p.ManufacturingCompanies, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
			); err != nil {
				return err
			}
			list = append(list, &p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: list products: %w", err)
	}
	return list, nil
}

// SetProductsStatus applies a status to many products at once and reports how
// many rows it actually touched.
//
// The count matters: row-level security silently filters out ids belonging to
// another tenant, so a caller passing fifty ids and seeing forty updated has
// learned that ten were not theirs — without ever seeing the rows.
func (r *Repository) SetProductsStatus(ctx context.Context, ids []int64, status catalog.ProductStatus) (int64, error) {
	var affected int64
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.products
			SET status = $2, updated_at = now()
			WHERE id = ANY($1) AND deleted_at IS NULL;
		`
		res, err := tx.Exec(txCtx, query, ids, string(status))
		if err != nil {
			return err
		}
		affected = res.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("catalog postgres: bulk status: %w", err)
	}
	return affected, nil
}

// DeleteCategory soft-deletes a category.
func (r *Repository) DeleteCategory(ctx context.Context, id int64) error {
	return r.softDeleteTaxonomy(ctx, "catalog.categories", "category", id)
}

// DeleteBrand soft-deletes a brand.
func (r *Repository) DeleteBrand(ctx context.Context, id int64) error {
	return r.softDeleteTaxonomy(ctx, "catalog.brands", "brand", id)
}

// softDeleteTaxonomy is shared by categories and brands, which are identical in
// shape. The table name is a package-internal constant at every call site, never
// caller input, so interpolating it does not open an injection path.
func (r *Repository) softDeleteTaxonomy(ctx context.Context, table, entity string, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE ` + table + `
			SET deleted_at = now(), status = 'inactive', updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;`
		res, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return fmt.Errorf("catalog postgres: delete %s: %w", entity, err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound(entity)
		}
		return nil
	})
}

// CountProductsInCategory counts live products referencing a category.
func (r *Repository) CountProductsInCategory(ctx context.Context, categoryID int64) (int, error) {
	return r.countProductsBy(ctx, "category_id", categoryID)
}

// CountProductsInBrand counts live products referencing a brand.
func (r *Repository) CountProductsInBrand(ctx context.Context, brandID int64) (int, error) {
	return r.countProductsBy(ctx, "brand_id", brandID)
}

func (r *Repository) countProductsBy(ctx context.Context, column string, id int64) (int, error) {
	var count int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT COUNT(*) FROM catalog.products
			WHERE ` + column + ` = $1 AND deleted_at IS NULL;`
		return tx.QueryRow(txCtx, query, id).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("catalog postgres: count products by %s: %w", column, err)
	}
	return count, nil
}
