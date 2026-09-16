package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// productHasStockSQL is "this product has at least one variant a pharmacy can order right now".
const productHasStockSQL = `EXISTS (
			      SELECT 1 FROM catalog.product_variants pv
			      JOIN inventory.stocks st ON st.product_variant_id = pv.id AND st.deleted_at IS NULL
			      WHERE pv.product_id = catalog.products.id
			        AND pv.deleted_at IS NULL
			        AND pv.status = 'active'
			        AND st.quantity > 0
			  )`

// stockFirstOrder puts orderable products ahead of the rest.
const stockFirstOrder = "(" + productHasStockSQL + ") DESC"

// productSearchPredicate is the text filter for SearchProducts, or the constant TRUE when there is no text to filter on.
func productSearchPredicate(query string) string {
	if strings.TrimSpace(query) == "" {
		return "TRUE"
	}
	return `(
			       platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($1) || '%'
			       OR name->>'en' ILIKE '%' || $1 || '%'
			       OR (scientific_name <> '' AND scientific_name ILIKE '%' || $1 || '%')
			       OR (active <> '' AND active ILIKE '%' || $1 || '%')
			       OR (manufacturing_companies <> '' AND manufacturing_companies ILIKE '%' || $1 || '%')
			       OR sku = $1
			       OR barcode = $1
			       OR platform.normalize_arabic($1) <% platform.normalize_arabic(name->>'ar')
			       OR platform.normalize_arabic(name->>'ar') % platform.normalize_arabic($1)
			       OR regexp_replace(platform.normalize_arabic(name->>'ar'), '[اوي]', '', 'g') ILIKE '%' || regexp_replace(platform.normalize_arabic($1), '[اوي]', '', 'g') || '%'
			       OR ($13 <> '' AND platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($13) || '%')
			       OR ($13 <> '' AND name->>'en' ILIKE '%' || $13 || '%')
			  )`
}

// SearchProducts performs fuzzy Arabic search and filters including institutional works.
func (r *Repository) SearchProducts(ctx context.Context, params catalog.SearchParams) ([]*catalog.Product, error) {
	var products []*catalog.Product
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if strings.TrimSpace(params.Query) != "" {
			if _, err := tx.Exec(txCtx, trgmThresholdsSQL); err != nil {
				return fmt.Errorf("catalog postgres: set trigram thresholds: %w", err)
			}
		}
		query := `
			SELECT id, public_id, organization_id, category_id, brand_id, branch_id,
			       name, description, sku, barcode, price, discount, old_price, image,
			       image_link, status, sold_times, is_featured, dosage_form,
			       scientific_name, pharmacology, active, concentration, unit,
			       manufacturing_companies, COALESCE(institutional_work_ids, '{}'::bigint[]),
			       created_at, updated_at, deleted_at
			FROM catalog.products
			WHERE deleted_at IS NULL
			  AND ` + productSearchPredicate(params.Query) + `
		  AND ($2::bigint IS NULL OR category_id = $2)
		  AND ($3::bigint IS NULL OR brand_id = $3)
		  AND ($6::numeric IS NULL OR price >= $6)
		  AND ($7::numeric IS NULL OR price <= $7)
		  AND ($10::text = '' OR status = $10)
		  AND ($11::text = '' OR dosage_form ILIKE '%' || $11 || '%')
		  AND ($14::text = '' OR COALESCE(manufacturing_companies, '') ILIKE '%' || $14 || '%')
			  AND (
			      ($8::int = 0 AND ($9::bigint[] IS NULL OR cardinality($9::bigint[]) = 0 OR cardinality(institutional_work_ids) = 0 OR institutional_work_ids && $9))
			      OR
			      ($8::int = 1 AND ($9::bigint[] IS NOT NULL AND cardinality($9::bigint[]) > 0 AND institutional_work_ids && $9))
			  )
			  AND ($12::boolean = false OR ` + productHasStockSQL + `)
		ORDER BY
			  ` + searchOrderPrefix(params.Query, params.Sort) + `
		  CASE
		    WHEN $1 = '' THEN 0
		    WHEN platform.normalize_arabic(name->>'ar') ILIKE platform.normalize_arabic($1) || '%' THEN 1
		    WHEN name->>'en' ILIKE $1 || '%' THEN 2
		    WHEN $13 <> '' AND platform.normalize_arabic(name->>'ar') ILIKE platform.normalize_arabic($13) || '%' THEN 3
		    WHEN $13 <> '' AND name->>'en' ILIKE $13 || '%' THEN 4
		    WHEN platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($1) || '%' THEN 5
		    WHEN name->>'en' ILIKE '%' || $1 || '%' THEN 6
		    WHEN $13 <> '' AND platform.normalize_arabic(name->>'ar') ILIKE '% ' || platform.normalize_arabic($13) || '%' THEN 7
		    WHEN COALESCE(scientific_name, '') ILIKE '%' || $1 || '%' THEN 8
		    WHEN COALESCE(active, '') ILIKE '%' || $1 || '%' OR COALESCE(manufacturing_companies, '') ILIKE '%' || $1 || '%' THEN 9
		    WHEN sku ILIKE '%' || $1 || '%' OR barcode ILIKE '%' || $1 || '%' THEN 10
		    ELSE 11
		  END,
			  ` + searchOrderSuffix(params.Query) + catalogOrderBy(params.Sort) + `
			LIMIT $4 OFFSET $5;
		`
		limit := params.Limit
		if limit <= 0 {
			limit = 50
		} else if limit > 1000 {
			limit = 1000
		}

		rows, err := tx.Query(txCtx, query,
			params.Query, params.CategoryID, params.BrandID, limit, params.Offset,
			params.MinPrice, params.MaxPrice, params.FilterMode, params.AllowedWorkIDs,
			params.Status, params.DosageForm, params.InStock, params.FirstWord,
			params.Manufacturer,
		)
		if err != nil {
			return fmt.Errorf("catalog postgres: search products: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var p catalog.Product
			var statusStr string
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.OrganizationID, &p.CategoryID, &p.BrandID, &p.BranchID,
				&p.Name, &p.Description, &p.SKU, &p.Barcode, &p.Price, &p.Discount,
				&p.OldPrice, &p.Image, &p.ImageLink, &statusStr, &p.SoldTimes, &p.IsFeatured,
				&p.DosageForm, &p.ScientificName, &p.Pharmacology, &p.Active,
				&p.Concentration, &p.Unit, &p.ManufacturingCompanies, &p.InstitutionalWorkIDs,
				&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
			); err != nil {
				return err
			}
			p.Status = catalog.ProductStatus(statusStr)
			products = append(products, &p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return products, nil
}

// CountProducts returns the total count of products matching search filters.
func (r *Repository) CountProducts(ctx context.Context, params catalog.SearchParams) (int, error) {
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT COUNT(*)
			FROM catalog.products
			WHERE deleted_at IS NULL
			  AND ($1 = '' 
			       OR platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($1) || '%'
			       OR name->>'en' ILIKE '%' || $1 || '%'
			       OR sku ILIKE '%' || $1 || '%'
			       OR barcode ILIKE '%' || $1 || '%'
			       OR COALESCE(scientific_name, '') ILIKE '%' || $1 || '%'
			       OR COALESCE(active, '') ILIKE '%' || $1 || '%'
			       OR COALESCE(manufacturing_companies, '') ILIKE '%' || $1 || '%'
		       OR word_similarity(platform.normalize_arabic($1), platform.normalize_arabic(name->>'ar')) >= 0.25
		       OR similarity(platform.normalize_arabic(name->>'ar'), platform.normalize_arabic($1)) >= 0.15
		       OR regexp_replace(platform.normalize_arabic(name->>'ar'), '[اوي]', '', 'g') ILIKE '%' || regexp_replace(platform.normalize_arabic($1), '[اوي]', '', 'g') || '%'
		       OR ($11 <> '' AND platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($11) || '%')
		       OR ($11 <> '' AND name->>'en' ILIKE '%' || $11 || '%'))
		  AND ($2::bigint IS NULL OR category_id = $2)
		  AND ($3::bigint IS NULL OR brand_id = $3)
		  AND ($4::numeric IS NULL OR price >= $4)
		  AND ($5::numeric IS NULL OR price <= $5)
		  AND ($6::text = '' OR status = $6)
		  AND ($7::text = '' OR dosage_form ILIKE '%' || $7 || '%')
		  AND ($12::text = '' OR COALESCE(manufacturing_companies, '') ILIKE '%' || $12 || '%')
			  AND (
			      ($8::int = 0 AND ($9::bigint[] IS NULL OR cardinality($9::bigint[]) = 0 OR cardinality(institutional_work_ids) = 0 OR institutional_work_ids && $9))
			      OR
			      ($8::int = 1 AND ($9::bigint[] IS NOT NULL AND cardinality($9::bigint[]) > 0 AND institutional_work_ids && $9))
			  )
			  AND ($10::boolean = false OR ` + productHasStockSQL + `);
		`
		return tx.QueryRow(txCtx, query,
			params.Query, params.CategoryID, params.BrandID,
			params.MinPrice, params.MaxPrice, params.Status, params.DosageForm,
			params.FilterMode, params.AllowedWorkIDs, params.InStock, params.FirstWord,
			params.Manufacturer,
		).Scan(&total)
	})
	return total, err
}
