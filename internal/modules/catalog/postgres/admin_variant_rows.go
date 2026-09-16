package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The cross-supplier stock listing's query.
//
// One statement replaces three: the page, a five-hundred-row organisation fetch
// and a thousand-row product fetch that were both used only to look up display
// names, and both silently wrong past their limit. It also carries the thing
// the screen was missing — the branch the offer sits on, and every warehouse
// holding it with its quantity.

// adminVariantFrom is the shared relation chain. The count and the page filter
// over identical relations or they disagree the moment a filter names a joined
// column.
const adminVariantFrom = `
	FROM catalog.product_variants v
	LEFT JOIN catalog.products p ON p.id = v.product_id
	LEFT JOIN org.organizations o ON o.id = v.organization_id
	LEFT JOIN org.branches b ON b.id = v.branch_id AND b.deleted_at IS NULL
	LEFT JOIN LATERAL (
		SELECT COALESCE(SUM(s.quantity), 0) AS qty,
		       COALESCE(MAX(s.min_threshold), 0) AS min_threshold
		FROM inventory.stocks s
		WHERE s.product_variant_id = v.id AND s.deleted_at IS NULL
	) st ON true`

// ListAdminVariantRows returns one filtered page of every supplier's stock.
func (r *Repository) ListAdminVariantRows(
	ctx context.Context, f catalog.AdminVariantFilter,
) ([]*catalog.AdminVariantRow, int, error) {
	f.Normalize()

	where := []string{"v.deleted_at IS NULL"}
	args := make([]any, 0, 10)
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if f.Status != "" {
		where = append(where, "v.status = "+arg(f.Status))
	}
	if f.ProductID > 0 {
		where = append(where, "v.product_id = "+arg(f.ProductID))
	}
	if f.OrganizationID > 0 {
		where = append(where, "v.organization_id = "+arg(f.OrganizationID))
	}
	if f.BranchID > 0 {
		where = append(where, "v.branch_id = "+arg(f.BranchID))
	}
	if f.WarehouseID > 0 {
		// A warehouse filter is a question about where the stock physically is,
		// which the rollup has already summed away. It needs its own EXISTS.
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM inventory.stocks s2
			WHERE s2.product_variant_id = v.id AND s2.deleted_at IS NULL
			  AND s2.warehouse_id = %s)`, arg(f.WarehouseID)))
	}
	switch f.Stock {
	case "in":
		where = append(where, "st.qty > 0")
	case "out":
		where = append(where, "st.qty <= 0")
	case "low":
		where = append(where, "st.qty > 0 AND st.qty <= GREATEST(st.min_threshold, 5)")
	}
	if f.ExpiringSoon {
		where = append(where, "v.expiry_date IS NOT NULL AND v.expiry_date <= (now() + INTERVAL '90 days')")
	}
	if f.Query != "" {
		p := arg("%" + f.Query + "%")
		where = append(where, fmt.Sprintf(`(
			v.name->>'ar' ILIKE %[1]s OR v.name->>'en' ILIKE %[1]s
			OR p.name->>'ar' ILIKE %[1]s OR p.name->>'en' ILIKE %[1]s
			OR v.sku ILIKE %[1]s OR v.barcode ILIKE %[1]s OR v.batch_number ILIKE %[1]s
			OR o.legal_name ILIKE %[1]s
			OR o.trade_name->>'ar' ILIKE %[1]s OR o.trade_name->>'en' ILIKE %[1]s)`, p))
	}

	whereSQL := strings.Join(where, " AND ")

	var out []*catalog.AdminVariantRow
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) `+adminVariantFrom+` WHERE `+whereSQL, args...).Scan(&total); err != nil {
			return fmt.Errorf("count admin variants: %w", err)
		}
		if total == 0 {
			return nil
		}

		paged := append(append([]any{}, args...), f.Limit, f.Offset)
		query := fmt.Sprintf(`
			SELECT v.id, COALESCE(v.product_id, 0), v.name,
			       COALESCE(p.name, '{}'::jsonb),
			       COALESCE(NULLIF(v.image, ''), COALESCE(p.image, '')),
			       (NULLIF(v.image, '') IS NULL AND COALESCE(p.image, '') <> ''),
			       COALESCE(v.sku, ''), COALESCE(v.barcode, ''), COALESCE(v.batch_number, ''),
			       v.status, v.price,
			       v.organization_id,
			       jsonb_build_object(
			           'ar', COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.name->>'ar', ''), NULLIF(o.legal_name, ''), NULLIF(o.trade_name->>'en', ''), NULLIF(o.name->>'en', ''), ''),
			           'en', COALESCE(NULLIF(o.trade_name->>'en', ''), NULLIF(o.name->>'en', ''), NULLIF(o.legal_name, ''), NULLIF(o.trade_name->>'ar', ''), NULLIF(o.name->>'ar', ''), '')
			       ),
			       COALESCE(o.type, ''),
			       v.branch_id, COALESCE(b.name, '{}'::jsonb),
			       COALESCE(st.qty, 0), COALESCE(st.min_threshold, 0),
			       COALESCE((
			           SELECT jsonb_agg(jsonb_build_object(
			                      'warehouse_id', w.id,
			                      'warehouse_name', COALESCE(w.name, ''),
			                      'branch_name', COALESCE(wb.name->>'ar', wb.name->>'en', ''),
			                      'quantity', s3.quantity)
			                  ORDER BY s3.quantity DESC)
			           FROM inventory.stocks s3
			           JOIN inventory.warehouses w ON w.id = s3.warehouse_id
			           LEFT JOIN org.branches wb ON wb.id = w.branch_id
			           WHERE s3.product_variant_id = v.id AND s3.deleted_at IS NULL
			             AND s3.quantity <> 0
			       ), '[]'::jsonb),
			       to_char(v.expiry_date, 'YYYY-MM-DD'),
			       COALESCE(v.min_order_qty, 1), COALESCE(v.quota_limit, 0)
			%s
			WHERE %s
			ORDER BY st.qty DESC, v.id DESC
			LIMIT $%d OFFSET $%d`,
			adminVariantFrom, whereSQL, len(args)+1, len(args)+2)

		rows, err := tx.Query(txCtx, query, paged...)
		if err != nil {
			return fmt.Errorf("list admin variants: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var row catalog.AdminVariantRow
			var warehousesJSON []byte
			var expiry *string
			var quotaLimit *int
			if err := rows.Scan(
				&row.VariantID, &row.ProductID, &row.VariantName, &row.ProductName,
				&row.Image, &row.ImageIsParent,
				&row.SKU, &row.Barcode, &row.BatchNumber, &row.Status, &row.Price,
				&row.OrganizationID, &row.OrganizationName, &row.OrganizationType,
				&row.BranchID, &row.BranchName,
				&row.TotalQuantity, &row.MinThreshold,
				&warehousesJSON, &expiry, &row.MinOrderQty, &quotaLimit,
			); err != nil {
				return fmt.Errorf("scan admin variant row: %w", err)
			}
			row.ExpiryDate = expiry
			if quotaLimit != nil {
				row.QuotaLimit = *quotaLimit
			}
			if len(warehousesJSON) > 0 {
				// A malformed aggregate is a row with no warehouse breakdown,
				// not a failed page: the totals above are still true.
				_ = json.Unmarshal(warehousesJSON, &row.Warehouses)
			}
			out = append(out, &row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, fmt.Errorf("catalog postgres: admin variant rows: %w", err)
	}
	return out, total, nil
}

// AdminVariantFilterOptions returns only the organisations, branches and
// warehouses that actually carry an offer.
//
// Listing every organisation on the platform would put pharmacies in a supplier
// filter and branches with no stock in a stock filter, which is a longer list
// that answers fewer questions.
func (r *Repository) AdminVariantFilterOptions(ctx context.Context) (catalog.AdminVariantOptions, error) {
	var opts catalog.AdminVariantOptions
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		orgRows, err := tx.Query(txCtx, `
			SELECT DISTINCT o.id,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.name->>'ar', ''),
			                NULLIF(o.trade_name->>'en', ''), NULLIF(o.name->>'en', ''),
			                o.legal_name, '')
			FROM org.organizations o
			JOIN catalog.product_variants v ON v.organization_id = o.id AND v.deleted_at IS NULL
			ORDER BY 2;`)
		if err != nil {
			return err
		}
		for orgRows.Next() {
			var opt catalog.AdminFilterOption
			if err := orgRows.Scan(&opt.ID, &opt.Label); err != nil {
				orgRows.Close()
				return err
			}
			opts.Organizations = append(opts.Organizations, opt)
		}
		orgRows.Close()
		if err := orgRows.Err(); err != nil {
			return err
		}

		branchRows, err := tx.Query(txCtx, `
			SELECT DISTINCT b.id, COALESCE(b.name->>'ar', b.name->>'en', ''), b.organization_id
			FROM org.branches b
			JOIN catalog.product_variants v ON v.branch_id = b.id AND v.deleted_at IS NULL
			WHERE b.deleted_at IS NULL
			ORDER BY 2;`)
		if err != nil {
			return err
		}
		for branchRows.Next() {
			var opt catalog.AdminFilterOption
			if err := branchRows.Scan(&opt.ID, &opt.Label, &opt.ParentID); err != nil {
				branchRows.Close()
				return err
			}
			opts.Branches = append(opts.Branches, opt)
		}
		branchRows.Close()
		if err := branchRows.Err(); err != nil {
			return err
		}

		whRows, err := tx.Query(txCtx, `
			SELECT DISTINCT w.id, COALESCE(w.name, ''), COALESCE(w.organization_id, 0)
			FROM inventory.warehouses w
			JOIN inventory.stocks s ON s.warehouse_id = w.id AND s.deleted_at IS NULL
			ORDER BY 2;`)
		if err != nil {
			return err
		}
		defer whRows.Close()
		for whRows.Next() {
			var opt catalog.AdminFilterOption
			if err := whRows.Scan(&opt.ID, &opt.Label, &opt.ParentID); err != nil {
				return err
			}
			opts.Warehouses = append(opts.Warehouses, opt)
		}
		return whRows.Err()
	})
	if err != nil {
		return catalog.AdminVariantOptions{}, fmt.Errorf("catalog postgres: admin variant filter options: %w", err)
	}
	return opts, nil
}
