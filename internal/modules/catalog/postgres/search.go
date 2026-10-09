package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
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

// productMatchSQL is the text filter. $1 is the query and $11 its first word.
//
// The page and its pager both read this one predicate (productFilterSQL). They
// used to carry two separately written copies: the page had the first-word
// branches and the trigram operators, the count had a regexp branch and
// different thresholds. A pager that counts other rows than the page lists
// leads to pages that are empty or missing results.
//
// Raw text is matched with strpos over lower(), not ILIKE. In an ILIKE pattern
// the admin's own '%' and '_' are wildcards, so typing "%" listed the whole
// catalogue. Normalised Arabic has those characters removed, so ILIKE is safe
// on it. The empty query is the first disjunct and matches every row cheaply.
const productMatchSQL = `(
		$1::text = ''
		OR platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($1) || '%'
		OR strpos(lower(name->>'en'), lower($1)) > 0
		OR strpos(lower(COALESCE(scientific_name, '')), lower($1)) > 0
		OR strpos(lower(COALESCE(active, '')), lower($1)) > 0
		OR strpos(lower(COALESCE(manufacturing_companies, '')), lower($1)) > 0
		OR strpos(lower(COALESCE(sku, '')), lower($1)) > 0
		OR strpos(lower(COALESCE(barcode, '')), lower($1)) > 0
		OR platform.normalize_arabic($1) <% platform.normalize_arabic(name->>'ar')
		OR platform.normalize_arabic(name->>'ar') % platform.normalize_arabic($1)
		OR regexp_replace(platform.normalize_arabic(name->>'ar'), '[اوي]', '', 'g')
		   ILIKE '%' || regexp_replace(platform.normalize_arabic($1), '[اوي]', '', 'g') || '%'
		OR ($11::text <> '' AND platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($11) || '%')
		OR ($11::text <> '' AND strpos(lower(name->>'en'), lower($11)) > 0)
	)`

// productFilterSQL is the WHERE body of both the page and its total. It binds
// $1 to $12, in the order productFilterArgs supplies them.
const productFilterSQL = `deleted_at IS NULL
  AND ` + productMatchSQL + `
  AND ($2::bigint IS NULL OR category_id = $2)
  AND ($3::bigint IS NULL OR brand_id = $3)
  AND ($4::numeric IS NULL OR price >= $4)
  AND ($5::numeric IS NULL OR price <= $5)
  AND ($6::text = '' OR status = $6)
  AND ($7::text = '' OR strpos(lower(COALESCE(dosage_form, '')), lower($7)) > 0)
  AND ($12::text = '' OR strpos(lower(COALESCE(manufacturing_companies, '')), lower($12)) > 0)
  AND (
      ($8::int = 0 AND ($9::bigint[] IS NULL OR cardinality($9::bigint[]) = 0 OR cardinality(institutional_work_ids) = 0 OR institutional_work_ids && $9))
      OR
      ($8::int = 1 AND ($9::bigint[] IS NOT NULL AND cardinality($9::bigint[]) > 0 AND institutional_work_ids && $9))
  )
  AND ($10::boolean = false OR ` + productHasStockSQL + `)`

// productFilterArgs binds the values productFilterSQL names, in the same order.
func productFilterArgs(params catalog.SearchParams) []any {
	return []any{
		params.Query, params.CategoryID, params.BrandID, params.MinPrice, params.MaxPrice,
		params.Status, params.DosageForm, params.FilterMode, params.AllowedWorkIDs,
		params.InStock, params.FirstWord, params.Manufacturer,
	}
}

// productListSQL is the page: the filter, the ranking, then LIMIT $13 OFFSET $14.
func productListSQL(params catalog.SearchParams) string {
	prefix, suffix := searchOrderPrefix(params.Query, params.Sort), searchOrderSuffix(params.Query)
	if params.SkipStockPriority {
		prefix, suffix = "", ""
	}
	return `
		SELECT id, public_id, organization_id, category_id, brand_id, branch_id,
		       name, description, sku, barcode, price, discount, old_price, image,
		       image_link, status, sold_times, is_featured, dosage_form,
		       scientific_name, pharmacology, active, concentration, unit,
		       manufacturing_companies, COALESCE(institutional_work_ids, '{}'::bigint[]),
		       created_at, updated_at, deleted_at
		FROM catalog.products
		WHERE ` + productFilterSQL + `
		ORDER BY
		  ` + prefix + `
		  CASE
		    WHEN $1::text = '' THEN 0
		    WHEN platform.normalize_arabic(name->>'ar') ILIKE platform.normalize_arabic($1) || '%' THEN 1
		    WHEN strpos(lower(name->>'en'), lower($1)) = 1 THEN 2
		    WHEN $11::text <> '' AND platform.normalize_arabic(name->>'ar') ILIKE platform.normalize_arabic($11) || '%' THEN 3
		    WHEN $11::text <> '' AND strpos(lower(name->>'en'), lower($11)) = 1 THEN 4
		    WHEN platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($1) || '%' THEN 5
		    WHEN strpos(lower(name->>'en'), lower($1)) > 0 THEN 6
		    WHEN $11::text <> '' AND platform.normalize_arabic(name->>'ar') ILIKE '% ' || platform.normalize_arabic($11) || '%' THEN 7
		    WHEN strpos(lower(COALESCE(scientific_name, '')), lower($1)) > 0 THEN 8
		    WHEN strpos(lower(COALESCE(active, '')), lower($1)) > 0
		      OR strpos(lower(COALESCE(manufacturing_companies, '')), lower($1)) > 0 THEN 9
		    WHEN strpos(lower(COALESCE(sku, '')), lower($1)) > 0
		      OR strpos(lower(COALESCE(barcode, '')), lower($1)) > 0 THEN 10
		    ELSE 11
		  END,
		  ` + suffix + catalogOrderBy(params.Sort) + `
		LIMIT $13 OFFSET $14;`
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
		limit := params.Limit
		if limit <= 0 {
			limit = 50
		} else if limit > 1000 {
			limit = 1000
		}

		args := append(productFilterArgs(params), limit, params.Offset)
		rows, err := tx.Query(txCtx, productListSQL(params), args...)
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
// It filters with the same predicate as SearchProducts, so the pager's total is
// the number of rows the pages can show.
func (r *Repository) CountProducts(ctx context.Context, params catalog.SearchParams) (int, error) {
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if strings.TrimSpace(params.Query) != "" {
			if _, err := tx.Exec(txCtx, trgmThresholdsSQL); err != nil {
				return fmt.Errorf("catalog postgres: set trigram thresholds: %w", err)
			}
		}
		query := "SELECT COUNT(*) FROM catalog.products WHERE " + productFilterSQL + ";"
		return tx.QueryRow(txCtx, query, productFilterArgs(params)...).Scan(&total)
	})
	return total, err
}
