package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// The vendor's own catalogue listing.
//
// One query answers the whole screen: the page of variants, the balance behind
// each of them, and the total the pager needs. The listing this replaces asked
// for five hundred rows, showed all of them at once, filtered them in the
// browser, and reported a stock of zero for every line — because
// catalog.product_variants has no stock column and nothing was joining the one
// that does.

// stockRollup totals a variant's balance across the vendor's warehouses.
//
// It is a lateral subquery rather than a join on the outer query so the
// aggregate is computed once per returned row instead of once per row in
// inventory.stocks, which matters on a catalogue of tens of thousands.
const stockRollup = `
	LEFT JOIN LATERAL (
		SELECT COALESCE(SUM(s.quantity), 0) AS qty,
		       COALESCE(MAX(s.min_threshold), 0) AS min_threshold,
		       COUNT(s.id) AS stock_count
		FROM inventory.stocks s
		WHERE s.product_variant_id = v.id AND s.deleted_at IS NULL
	) st ON true`

// vendorVariantColumns is the projection the listing shares.
const vendorVariantColumns = `
	v.id, v.public_id, v.organization_id, COALESCE(v.product_id, 0), v.name, v.sku,
	v.barcode, v.price, v.cost_price, COALESCE(v.cost_discount_percentage, 0.00), v.discount, v.unit, v.image, v.status,
	v.is_featured, v.is_negotiable, v.batch_number, v.expiry_date, v.min_order_qty,
	v.quota_limit, v.branch_id, v.created_at, v.updated_at, st.qty`

// ListVendorVariants returns one page of a vendor's variants with their stock.
func (r *Repository) ListVendorVariants(
	ctx context.Context, orgID int64, params catalog.VendorVariantQuery,
) ([]*catalog.ProductVariant, int, error) {
	where, args := vendorVariantFilter(orgID, params)
	limit, offset := params.Page()

	var out []*catalog.ProductVariant
	var total int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		countSQL := `SELECT count(*) FROM catalog.product_variants v ` + stockRollup +
			` WHERE ` + where
		if err := tx.QueryRow(txCtx, countSQL, args...).Scan(&total); err != nil {
			return fmt.Errorf("catalog postgres: count vendor variants: %w", err)
		}
		if total == 0 {
			return nil
		}

		paged := append(append([]any{}, args...), limit, offset)
		listSQL := fmt.Sprintf(`SELECT %s FROM catalog.product_variants v %s WHERE %s
			ORDER BY %s LIMIT $%d OFFSET $%d`,
			vendorVariantColumns, stockRollup, where, vendorVariantOrder(params.Sort),
			len(args)+1, len(args)+2)

		rows, err := tx.Query(txCtx, listSQL, paged...)
		if err != nil {
			return fmt.Errorf("catalog postgres: list vendor variants: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.ProductVariant
			var status string
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
				&v.Barcode, &v.Price, &v.CostPrice, &v.CostDiscountPercentage, &v.Discount, &v.Unit, &v.Image,
				&status, &v.IsFeatured, &v.IsNegotiable, &v.BatchNumber, &v.ExpiryDate,
				&v.MinOrderQty, &v.QuotaLimit, &v.BranchID, &v.CreatedAt, &v.UpdatedAt, &v.StockQty,
			); err != nil {
				return fmt.Errorf("catalog postgres: scan vendor variant: %w", err)
			}
			v.Status = catalog.ProductStatus(status)
			out = append(out, &v)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// vendorVariantFilter builds the shared WHERE clause.
func vendorVariantFilter(orgID int64, params catalog.VendorVariantQuery) (string, []any) {
	clauses := []string{"v.organization_id = $1", "v.deleted_at IS NULL"}
	args := []any{orgID}

	if params.Status != "" {
		args = append(args, params.Status)
		clauses = append(clauses, fmt.Sprintf("v.status = $%d", len(args)))
	}
	if q := strings.TrimSpace(params.Query); q != "" {
		args = append(args, "%"+q+"%")
		n := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(v.name->>'ar' ILIKE $%d OR v.name->>'en' ILIKE $%d OR v.sku ILIKE $%d"+
				" OR v.barcode ILIKE $%d OR v.batch_number ILIKE $%d)", n, n, n, n, n))
	}
	switch params.Stock {
	case catalog.StockFilterIn:
		clauses = append(clauses, "st.stock_count > 0 AND st.qty > 0")
	case catalog.StockFilterOut:
		clauses = append(clauses, "st.stock_count > 0 AND st.qty <= 0")
	case catalog.StockFilterLow:
		// "Low" means at or below the threshold the vendor set for that
		// warehouse, falling back to five where they set none.
		clauses = append(clauses, "st.stock_count > 0 AND st.qty > 0 AND st.qty <= GREATEST(st.min_threshold, 5)")
	}
	if params.Expiring {
		clauses = append(clauses, "v.expiry_date IS NOT NULL AND v.expiry_date <= (now() + INTERVAL '90 days')")
	}
	return strings.Join(clauses, " AND "), args
}

// vendorVariantOrder maps a sort key onto a deterministic ordering.
//
// Every ordering ends in the id so a page boundary cannot show one row twice
// and skip another, which is what happens when a paged query sorts on a column
// with duplicates.
func vendorVariantOrder(sort string) string {
	switch sort {
	case "name":
		return "v.name->>'ar' ASC, v.id DESC"
	case "price_asc":
		return "v.price ASC, v.id DESC"
	case "price_desc":
		return "v.price DESC, v.id DESC"
	case "stock_asc":
		return "st.qty ASC, v.id DESC"
	case "stock_desc":
		return "st.qty DESC, v.id DESC"
	case "expiry":
		return "v.expiry_date ASC NULLS LAST, v.id DESC"
	case "oldest":
		return "v.id ASC"
	default:
		return "v.id DESC"
	}
}

// VendorVariantStats counts the whole catalogue, not the page.
//
// The screen used to compute its headline figures from whatever rows happened
// to be loaded, so a vendor with nine thousand variants was told they had five
// hundred. These are the real numbers.
func (r *Repository) VendorVariantStats(
	ctx context.Context, orgID int64,
) (catalog.VendorVariantStats, error) {
	var stats catalog.VendorVariantStats
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT count(*),
			       count(*) FILTER (WHERE v.status = 'active'),
			       count(*) FILTER (WHERE st.stock_count > 0 AND st.qty > 0),
			       count(*) FILTER (WHERE st.stock_count > 0 AND st.qty > 0 AND st.qty <= GREATEST(st.min_threshold, 5)),
			       count(*) FILTER (WHERE st.stock_count > 0 AND st.qty <= 0),
			       count(*) FILTER (WHERE v.expiry_date IS NOT NULL
			                          AND v.expiry_date <= (now() + INTERVAL '90 days'))
			FROM catalog.product_variants v `+stockRollup+`
			WHERE v.organization_id = $1 AND v.deleted_at IS NULL`, orgID,
		).Scan(&stats.Total, &stats.Active, &stats.InStock, &stats.LowStock,
			&stats.OutOfStock, &stats.Expiring)
	})
	if err != nil {
		return stats, fmt.Errorf("catalog postgres: vendor variant stats: %w", err)
	}
	return stats, nil
}

// ListProductsByIDs loads many catalogue products in one query.
//
// The listing needs the shared-catalogue name, image and manufacturer for every
// row on the page. Fetching those one at a time is a hundred round trips per
// page view; this is one.
func (r *Repository) ListProductsByIDs(
	ctx context.Context, ids []int64,
) (map[int64]*catalog.Product, error) {
	out := make(map[int64]*catalog.Product, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT id, public_id, organization_id, category_id, brand_id, name,
			       sku, barcode, price, discount, old_price, image, status,
			       dosage_form, scientific_name, concentration, unit,
			       manufacturing_companies
			FROM catalog.products
			WHERE id = ANY($1) AND deleted_at IS NULL`, ids)
		if err != nil {
			return fmt.Errorf("catalog postgres: list products by ids: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var p catalog.Product
			var status string
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.OrganizationID, &p.CategoryID, &p.BrandID,
				&p.Name, &p.SKU, &p.Barcode, &p.Price, &p.Discount, &p.OldPrice,
				&p.Image, &status, &p.DosageForm, &p.ScientificName,
				&p.Concentration, &p.Unit, &p.ManufacturingCompanies,
			); err != nil {
				return fmt.Errorf("catalog postgres: scan product: %w", err)
			}
			p.Status = catalog.ProductStatus(status)
			out[p.ID] = &p
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
