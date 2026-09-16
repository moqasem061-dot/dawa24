package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// UpsertProductIndex inserts or updates a read-model item in catalog.product_index.
func (r *Repository) UpsertProductIndex(ctx context.Context, item *catalog.ProductIndexItem) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.product_index (
				unique_row_id, product_id, variant_id, sku, name_ar, name_en,
				search_text, search_ar, search_en, search_simple,
				organization_name, branch_city, scientific_name, price, discount,
				stock_quantity, category_id, brand_id, has_discount,
				discount_percentage, price_after_discount, organization_id, branch_id,
				status, product_type, institutional_work_ids, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
				$16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, now()
			)
			ON CONFLICT (unique_row_id) DO UPDATE SET
				sku = EXCLUDED.sku,
				name_ar = EXCLUDED.name_ar,
				name_en = EXCLUDED.name_en,
				search_text = EXCLUDED.search_text,
				search_ar = EXCLUDED.search_ar,
				search_en = EXCLUDED.search_en,
				search_simple = EXCLUDED.search_simple,
				organization_name = EXCLUDED.organization_name,
				branch_city = EXCLUDED.branch_city,
				scientific_name = EXCLUDED.scientific_name,
				price = EXCLUDED.price,
				discount = EXCLUDED.discount,
				stock_quantity = EXCLUDED.stock_quantity,
				category_id = EXCLUDED.category_id,
				brand_id = EXCLUDED.brand_id,
				has_discount = EXCLUDED.has_discount,
				discount_percentage = EXCLUDED.discount_percentage,
				price_after_discount = EXCLUDED.price_after_discount,
				organization_id = EXCLUDED.organization_id,
				branch_id = EXCLUDED.branch_id,
				status = EXCLUDED.status,
				product_type = EXCLUDED.product_type,
				institutional_work_ids = EXCLUDED.institutional_work_ids,
				updated_at = now();
		`
		_, err := tx.Exec(txCtx, query,
			item.UniqueRowID, item.ProductID, item.VariantID, item.SKU,
			item.NameAR, item.NameEN, item.SearchText, item.SearchAR, item.SearchEN, item.SearchSimple,
			item.OrganizationName, item.BranchCity, item.ScientificName, item.Price, item.Discount,
			item.StockQuantity, item.CategoryID, item.BrandID, item.HasDiscount,
			item.DiscountPercentage, item.PriceAfterDiscount, item.OrganizationID, item.BranchID,
			item.Status, item.ProductType, item.InstitutionalWorkIDs,
		)
		if err != nil {
			return fmt.Errorf("catalog postgres: upsert product_index: %w", err)
		}
		return nil
	})
}

// DeleteProductIndex removes a single read model entry by unique_row_id.
func (r *Repository) DeleteProductIndex(ctx context.Context, uniqueRowID string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM catalog.product_index WHERE unique_row_id = $1;`
		_, err := tx.Exec(txCtx, query, uniqueRowID)
		if err != nil {
			return fmt.Errorf("catalog postgres: delete product_index: %w", err)
		}
		return nil
	})
}

// DeleteProductIndexByProduct removes all index entries for a master product.
func (r *Repository) DeleteProductIndexByProduct(ctx context.Context, productID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM catalog.product_index WHERE product_id = $1;`
		_, err := tx.Exec(txCtx, query, productID)
		if err != nil {
			return fmt.Errorf("catalog postgres: delete product_index by product: %w", err)
		}
		return nil
	})
}

// productIndexColumns is the projection every product_index read shares.
const productIndexColumns = `unique_row_id, product_id, variant_id, sku, name_ar, name_en,
	       search_text, search_ar, search_en, search_simple,
	       organization_name, branch_city, scientific_name, price, discount,
	       stock_quantity, category_id, brand_id, has_discount,
	       discount_percentage, price_after_discount, organization_id, branch_id,
	       status, product_type, COALESCE(institutional_work_ids, '{}'::bigint[]),
	       created_at, updated_at`

// trgmThresholdsSQL sets the trigram cut-offs the search predicate relies on.
//
// They reproduce exactly the numbers the previous query hard-coded as function
// comparisons — word_similarity(...) >= 0.25 and similarity(...) >= 0.15 — but
// as session settings, which is what lets the same comparison be written with
// the <% and % operators instead. That difference is the entire point: a GIN
// trigram index can answer the operators and cannot answer the function calls,
// so the old form obliged PostgreSQL to read every row to evaluate them.
//
// SET LOCAL, so the values are scoped to this transaction and cannot leak onto
// whatever request borrows the connection next.
const trgmThresholdsSQL = `SET LOCAL pg_trgm.similarity_threshold = 0.15;
	SET LOCAL pg_trgm.word_similarity_threshold = 0.25;`

// searchPredicate matches a row against the caller's text. %[1]s is the
// placeholder holding the query.
//
// Every branch is answerable from an index — the GIN tsvector index for the
// first, the GIN trigram indexes on search_simple and search_text for the rest
// — so PostgreSQL combines them with a BitmapOr instead of scanning.
//
// That property is both fragile and load-bearing. ONE unindexable branch in an
// OR forces a sequential scan of the whole table however good the others are,
// and that is precisely what the previous version did: it added ILIKE branches
// against name_ar, name_en, sku and scientific_name, none of which carry a
// trigram index. Measured on this database against 19,996 rows, searching
// "panadol" took 780 ms and read 2,810 buffers from disk, while the 8.8 MB
// full-text index it should have used had recorded zero scans since it was
// built. The same search now plans as a BitmapOr and runs in 19 ms; a term
// matching nothing — the worst case, because nothing lets it stop early —
// runs in 2.4 ms instead of scanning the table.
//
// The removed column branches are not lost recall. search_simple already holds
// the normalised Arabic name, the English name and the SKU; search_text adds
// the scientific name, the pharmacology, the manufacturer and the supplier's
// own name. Between them they cover every column the old chain named.
//
// Before adding a branch here, confirm an index can answer it — with EXPLAIN,
// not by inspection.
const searchPredicate = `(
			       search_vector @@ plainto_tsquery('simple', %[1]s)
			       OR search_simple ILIKE '%%' || platform.normalize_arabic(%[1]s) || '%%'
			       OR search_text ILIKE '%%' || %[1]s || '%%'
			       OR platform.normalize_arabic(%[1]s) <%% search_simple
			       OR search_simple %% platform.normalize_arabic(%[1]s)
			  )`

// searchRank puts exact and prefix hits ahead of fuzzy ones. %[1]s is the
// placeholder holding the query.
//
// It stays an expression over ILIKE, which is not indexable — and does not need
// to be. It is evaluated only on the rows the predicate above already selected,
// which is hundreds rather than twenty thousand. The previous version evaluated
// the same shape over every row in the table before sorting.
const searchRank = `CASE
			    WHEN search_simple ILIKE platform.normalize_arabic(%[1]s) || '%%' THEN 1
			    WHEN search_en ILIKE %[1]s || '%%' THEN 2
			    WHEN search_simple ILIKE '%%' || platform.normalize_arabic(%[1]s) || '%%' THEN 3
			    WHEN search_en ILIKE '%%' || %[1]s || '%%' THEN 4
			    ELSE 5
			  END`

// SearchProductIndex queries the denormalized read model with fast fulltext, trigrams, and institutional filtering.
// Uses database.AsSystem because cross-tenant catalogue discovery is the core function of the multi-vendor marketplace.
//
// The listing case and the search case are built as two different statements
// rather than as one statement with an `OR $1 = ”` escape hatch.
//
// That escape was not free. A parameter compared against a constant cannot be
// folded away when PostgreSQL builds a generic plan, so one plan had to serve
// both "list the catalogue" and "search the catalogue" — and the only plan that
// serves both is a sequential scan. Building them separately lets each get the
// plan it deserves: an ordered index walk for the listing, a BitmapOr over the
// text indexes for the search.
//
// The filters are appended only when set, for the same reason, instead of being
// written as `($n IS NULL OR col = $n)`: the null-guard form asks the planner
// to produce one plan covering every combination of filters any caller might
// pass, rather than a plan for the filters actually in use.
func (r *Repository) SearchProductIndex(ctx context.Context, params catalog.SearchParams) ([]*catalog.ProductIndexItem, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	} else if limit > 1000 {
		limit = 1000
	}

	var args []any
	// arg registers a bind value and returns the placeholder that names it.
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	where := []string{"status = 'active'"}

	searching := params.Query != ""
	var queryArg string
	if searching {
		queryArg = arg(params.Query)
		where = append(where, fmt.Sprintf(searchPredicate, queryArg))
	}
	if params.CategoryID != nil {
		where = append(where, "category_id = "+arg(*params.CategoryID))
	}
	if params.BrandID != nil {
		where = append(where, "brand_id = "+arg(*params.BrandID))
	}
	if params.MinPrice != nil {
		where = append(where, "price_after_discount >= "+arg(*params.MinPrice))
	}
	if params.MaxPrice != nil {
		where = append(where, "price_after_discount <= "+arg(*params.MaxPrice))
	}

	// Institutional visibility, unchanged in meaning. Mode 1 demands an overlap
	// with the caller's permitted works; mode 0 admits a row that is either
	// unrestricted or overlapping.
	modeArg, worksArg := arg(params.FilterMode), arg(params.AllowedWorkIDs)
	where = append(where, fmt.Sprintf(`(
			      (%[1]s::int = 0 AND (%[2]s::bigint[] IS NULL OR cardinality(%[2]s::bigint[]) = 0 OR cardinality(institutional_work_ids) = 0 OR institutional_work_ids && %[2]s))
			      OR
			      (%[1]s::int = 1 AND (%[2]s::bigint[] IS NOT NULL AND cardinality(%[2]s::bigint[]) > 0 AND institutional_work_ids && %[2]s))
			  )`, modeArg, worksArg))

	orderBy := "price_after_discount ASC, updated_at DESC"
	if searching {
		orderBy = fmt.Sprintf(searchRank, queryArg) + ",\n\t\t\t  " + orderBy
	}

	query := "SELECT " + productIndexColumns + "\n\t\t\tFROM catalog.product_index" +
		"\n\t\t\tWHERE " + strings.Join(where, "\n\t\t\t  AND ") +
		"\n\t\t\tORDER BY " + orderBy +
		"\n\t\t\tLIMIT " + arg(limit) + " OFFSET " + arg(params.Offset) + ";"

	items := make([]*catalog.ProductIndexItem, 0, limit)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if searching {
			if _, err := tx.Exec(txCtx, trgmThresholdsSQL); err != nil {
				return fmt.Errorf("catalog postgres: set trigram thresholds: %w", err)
			}
		}

		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return fmt.Errorf("catalog postgres: search product_index: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var it catalog.ProductIndexItem
			if err := rows.Scan(
				&it.UniqueRowID, &it.ProductID, &it.VariantID, &it.SKU,
				&it.NameAR, &it.NameEN, &it.SearchText, &it.SearchAR, &it.SearchEN, &it.SearchSimple,
				&it.OrganizationName, &it.BranchCity, &it.ScientificName, &it.Price, &it.Discount,
				&it.StockQuantity, &it.CategoryID, &it.BrandID, &it.HasDiscount,
				&it.DiscountPercentage, &it.PriceAfterDiscount, &it.OrganizationID, &it.BranchID,
				&it.Status, &it.ProductType, &it.InstitutionalWorkIDs,
				&it.CreatedAt, &it.UpdatedAt,
			); err != nil {
				return err
			}
			items = append(items, &it)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// rebuildIndexLockKey serialises index rebuilds across every process. Any
// constant works; this one is arbitrary and unique to this operation.
const rebuildIndexLockKey int64 = 0x6361_7461_6c6f_6701

// RebuildProductIndex truncates and fully repopulates catalog.product_index from master tables.
//
// Two protections wrap the rebuild, both learned from it deadlocking against an
// ordinary product write:
//
//   - An advisory lock, so two rebuilds — an import finishing while a scheduled
//     sweep runs — cannot pile up on the same TRUNCATE.
//   - A lock timeout, because this transaction takes ACCESS EXCLUSIVE on
//     product_index and then reads catalog.products, while a DELETE on
//     catalog.products locks them in the opposite order through the index's
//     ON DELETE SET NULL. That inversion is a deadlock waiting for traffic. The
//     timeout makes the rebuild the side that gives way: it is a derived table
//     that the next import or sweep will rebuild anyway, whereas the write it
//     would otherwise kill is somebody's catalogue edit.
func (r *Repository) RebuildProductIndex(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `SET LOCAL lock_timeout = '5s'`); err != nil {
			return fmt.Errorf("set rebuild lock timeout: %w", err)
		}
		if _, err := tx.Exec(txCtx, `SELECT pg_advisory_xact_lock($1)`, rebuildIndexLockKey); err != nil {
			return fmt.Errorf("acquire rebuild lock: %w", err)
		}
		if _, err := tx.Exec(txCtx, `TRUNCATE TABLE catalog.product_index;`); err != nil {
			return fmt.Errorf("truncate product_index: %w", err)
		}

		query := `
			INSERT INTO catalog.product_index (
				unique_row_id, product_id, variant_id, sku, name_ar, name_en,
				search_text, search_ar, search_en, search_simple,
				organization_name, branch_city, scientific_name, price, discount,
				stock_quantity, category_id, brand_id, has_discount,
				discount_percentage, price_after_discount, organization_id, branch_id,
				status, product_type, institutional_work_ids
			)
			SELECT 
				'p_' || p.id::text AS unique_row_id,
				p.id AS product_id,
				NULL::bigint AS variant_id,
				p.sku,
				p.name->>'ar' AS name_ar,
				p.name->>'en' AS name_en,
				CONCAT_WS(' ', COALESCE(p.name->>'ar', ''), COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.name->>'ar', ''), COALESCE(p.sku, '')) AS search_text,
				CONCAT_WS(' ', COALESCE(p.name->>'ar', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.name->>'ar', '')) AS search_ar,
				CONCAT_WS(' ', COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.name->>'en', '')) AS search_en,
				CONCAT_WS(' ', platform.normalize_arabic(COALESCE(p.name->>'ar', '')), COALESCE(p.name->>'en', ''), COALESCE(p.sku, '')) AS search_simple,
				COALESCE(o.name->>'ar', o.name->>'en', 'دوا 24') AS organization_name,
				COALESCE(b.name->>'ar', 'المخزن الرئيسي') AS branch_city,
				p.scientific_name,
				p.price,
				p.discount,
				0 AS stock_quantity,
				p.category_id,
				p.brand_id,
				(p.discount > 0) AS has_discount,
				p.discount AS discount_percentage,
				ROUND(p.price * (1 - p.discount / 100), 2) AS price_after_discount,
				p.organization_id,
				p.branch_id,
				p.status::text AS status,
				'parent' AS product_type,
				COALESCE(p.institutional_work_ids, '{}'::bigint[])
			FROM catalog.products p
			JOIN org.organizations o ON p.organization_id = o.id
			LEFT JOIN org.branches b ON p.branch_id = b.id
			WHERE p.deleted_at IS NULL AND o.deleted_at IS NULL AND p.status = 'active';
		`
		res, err := tx.Exec(txCtx, query)
		if err != nil {
			return fmt.Errorf("rebuild product_index: %w", err)
		}
		count = res.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
