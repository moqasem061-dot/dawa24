package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// catalog.brand_categories is reference data with no organization_id and no
// RLS, matching catalog.categories and catalog.brands: the product catalogue is
// shared across tenants by design, because a pharmacy browsing suppliers has to
// see it. AsSystem is used so the reads are not filtered by a tenant GUC that
// does not apply.

// ListBrandsByCategory returns the manufacturers linked to one category.
func (r *Repository) ListBrandsByCategory(ctx context.Context, categoryID int64) ([]*catalog.Brand, error) {
	var brands []*catalog.Brand
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const q = `
			SELECT b.id, b.public_id, b.name, b.description, b.image, b.status,
			       b.created_at, b.updated_at
			FROM catalog.brands b
			JOIN catalog.brand_categories bc ON bc.brand_id = b.id
			WHERE bc.category_id = $1
			  AND b.deleted_at IS NULL
			  AND b.status = 'active'
			ORDER BY b.name->>'ar' ASC;
		`
		rows, err := tx.Query(txCtx, q, categoryID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b catalog.Brand
			if err := rows.Scan(&b.ID, &b.PublicID, &b.Name, &b.Description, &b.Image,
				&b.Status, &b.CreatedAt, &b.UpdatedAt); err != nil {
				return err
			}
			brands = append(brands, &b)
		}
		return rows.Err()
	})
	return brands, err
}

// BrandInCategory is the server-side check behind the cascading selector.
func (r *Repository) BrandInCategory(ctx context.Context, categoryID, brandID int64) (bool, error) {
	var ok bool
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const q = `SELECT EXISTS (SELECT 1 FROM catalog.brand_categories WHERE category_id = $1 AND brand_id = $2)`
		return tx.QueryRow(txCtx, q, categoryID, brandID).Scan(&ok)
	})
	return ok, err
}

// SetBrandCategories replaces a manufacturer's category set in one transaction,
// so a half-applied edit cannot leave the brand in categories it no longer
// belongs to.
func (r *Repository) SetBrandCategories(ctx context.Context, brandID int64, categoryIDs []int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// PERF OPTIMIZATION NOTE (Priority 10):
		// Iterating categoryIDs issues N separate INSERT statements within the transaction.
		// Future refactor: bulk-insert in a single statement using:
		// INSERT INTO catalog.brand_categories (brand_id, category_id)
		// SELECT $1, unnest($2::bigint[]) ON CONFLICT DO NOTHING;
		for _, cid := range categoryIDs {
			if cid <= 0 {
				continue
			}
			const q = `INSERT INTO catalog.brand_categories (brand_id, category_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
			if _, err := tx.Exec(txCtx, q, brandID, cid); err != nil {
				return err
			}
		}
		return nil
	})
}
