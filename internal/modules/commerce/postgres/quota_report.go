package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The supplier's quota screen.
//
// Every figure here is derived from the same predicate the purchase gate uses
// (quotaCountsSQL), so what the supplier reads is exactly what a pharmacy will
// be told at the cart. A separate reporting query with its own idea of which
// orders count is how a screen comes to say "3 of 10" while the buyer is
// refused at 4.
//
// The reads are AsSystem for the obvious reason: the rows being summed are the
// *buyers'* orders and the *buyers'* branches. A supplier is entitled to know
// how much of its own restricted item each branch has taken — that is the whole
// point of the quota — and nothing else about those orders crosses over.

// quotaConsumptionCTE is the shared body: one row per (variant, branch) that
// has ever bought a quota-carrying variant of this supplier, with the totals
// that still count.
//
// $1 is the supplier's organisation id and $2 the releasing-status list. A
// caller appends its own filters and its own arguments after those two.
const quotaConsumptionCTE = `
	WITH consumption AS (
		SELECT ol.product_variant_id            AS variant_id,
		       o.branch_id                      AS branch_id,
		       SUM(ol.quantity)                 AS used,
		       COUNT(DISTINCT o.id)             AS order_count,
		       MAX(o.created_at)                AS last_order_at
		FROM commerce.order_lines ol
		JOIN commerce.orders o ON o.id = ol.order_id
		JOIN catalog.product_variants v ON v.id = ol.product_variant_id` + quotaJoinSQL + `
		WHERE ol.organization_id = $1
		  AND v.organization_id = $1
		  AND v.deleted_at IS NULL
		  AND v.quota_limit IS NOT NULL
		  AND ol.product_variant_id IS NOT NULL
		  AND o.branch_id IS NOT NULL
		  AND ` + quotaCountsSQL + `
		GROUP BY ol.product_variant_id, o.branch_id
	),
	released AS (
		SELECT rel.product_variant_id            AS variant_id,
		       rel.branch_id                     AS branch_id,
		       rel.released_at,
		       rel.release_count,
		       COALESCE((
		           SELECT SUM(ol2.quantity)
		           FROM commerce.order_lines ol2
		           JOIN commerce.orders o2 ON o2.id = ol2.order_id
		           WHERE ol2.product_variant_id = rel.product_variant_id
		             AND o2.branch_id = rel.branch_id
		             AND o2.deleted_at IS NULL
		             AND o2.status <> ALL ($2)
		             AND o2.created_at <= rel.released_at
		       ), 0)                             AS released_units
		FROM commerce.variant_branch_quota_releases rel
		JOIN catalog.product_variants rv ON rv.id = rel.product_variant_id
		WHERE rel.organization_id = $1
		  AND rv.deleted_at IS NULL
		  AND rv.quota_limit IS NOT NULL
	),
	pairings AS (
		SELECT variant_id, branch_id FROM consumption
		UNION
		SELECT variant_id, branch_id FROM released
	)`

// quotaRowSelect turns one pairing into a screen row.
const quotaRowSelect = `
	SELECT p.variant_id,
	       COALESCE(NULLIF(v.name->>'ar', ''), NULLIF(v.name->>'en', ''), v.sku, '') AS variant_name,
	       COALESCE(v.product_id, 0),
	       COALESCE(NULLIF(pr.name->>'ar', ''), NULLIF(pr.name->>'en', ''), '')      AS product_name,
	       COALESCE(v.sku, ''),
	       COALESCE(v.quota_limit, 0),
	       p.branch_id,
	       COALESCE(NULLIF(b.name->>'ar', ''), NULLIF(b.name->>'en', ''), b.code, '') AS branch_name,
	       COALESCE(b.organization_id, 0),
	       COALESCE(NULLIF(co.name->>'ar', ''), NULLIF(co.name->>'en', ''), '')      AS customer_name,
	       COALESCE(c.used, 0)::int,
	       COALESCE(c.order_count, 0)::int,
	       c.last_order_at,
	       rl.released_at,
	       COALESCE(rl.release_count, 0)::int,
	       COALESCE(rl.released_units, 0)::int
	FROM pairings p
	JOIN catalog.product_variants v ON v.id = p.variant_id
	LEFT JOIN catalog.products pr ON pr.id = v.product_id
	LEFT JOIN org.branches b ON b.id = p.branch_id
	LEFT JOIN org.organizations co ON co.id = b.organization_id
	LEFT JOIN consumption c ON c.variant_id = p.variant_id AND c.branch_id = p.branch_id
	LEFT JOIN released rl ON rl.variant_id = p.variant_id AND rl.branch_id = p.branch_id`

// quotaRowFilters appends the screen's filters, numbering its parameters from
// the position the caller has already reached.
func quotaRowFilters(f commerce.QuotaFilter, args []any) (string, []any) {
	var clauses []string
	if f.VariantID > 0 {
		args = append(args, f.VariantID)
		clauses = append(clauses, fmt.Sprintf("p.variant_id = $%d", len(args)))
	}
	if f.CustomerOrgID > 0 {
		args = append(args, f.CustomerOrgID)
		clauses = append(clauses, fmt.Sprintf("b.organization_id = $%d", len(args)))
	}
	if f.BranchID > 0 {
		args = append(args, f.BranchID)
		clauses = append(clauses, fmt.Sprintf("p.branch_id = $%d", len(args)))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+q+"%")
		n := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(v.name->>'ar' ILIKE $%d OR v.name->>'en' ILIKE $%d OR v.sku ILIKE $%d"+
				" OR pr.name->>'ar' ILIKE $%d OR pr.name->>'en' ILIKE $%d"+
				" OR b.name->>'ar' ILIKE $%d OR b.name->>'en' ILIKE $%d"+
				" OR co.name->>'ar' ILIKE $%d OR co.name->>'en' ILIKE $%d)",
			n, n, n, n, n, n, n, n, n))
	}
	switch f.State {
	case commerce.QuotaStateExhausted:
		clauses = append(clauses, "COALESCE(c.used, 0) >= COALESCE(v.quota_limit, 0)")
	case commerce.QuotaStateActive:
		clauses = append(clauses, "COALESCE(c.used, 0) < COALESCE(v.quota_limit, 0)")
	case commerce.QuotaStateReleased:
		clauses = append(clauses, "rl.released_at IS NOT NULL")
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// ListBranchQuotaRows returns one page of "which branch took how much of what".
func (r *Repository) ListBranchQuotaRows(
	ctx context.Context, vendorOrgID int64, f commerce.QuotaFilter,
) ([]*commerce.BranchQuotaRow, int, error) {
	if vendorOrgID <= 0 {
		return nil, 0, nil
	}
	base := replaceReleasing(quotaConsumptionCTE, "$2")
	args := []any{vendorOrgID, commerce.QuotaReleasingStatusStrings()}
	where, args := quotaRowFilters(f, args)

	var rowsOut []*commerce.BranchQuotaRow
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countSQL := base + `
			SELECT count(*)
			FROM pairings p
			JOIN catalog.product_variants v ON v.id = p.variant_id
			LEFT JOIN catalog.products pr ON pr.id = v.product_id
			LEFT JOIN org.branches b ON b.id = p.branch_id
			LEFT JOIN org.organizations co ON co.id = b.organization_id
			LEFT JOIN consumption c ON c.variant_id = p.variant_id AND c.branch_id = p.branch_id
			LEFT JOIN released rl ON rl.variant_id = p.variant_id AND rl.branch_id = p.branch_id` + where
		if err := tx.QueryRow(txCtx, countSQL, args...).Scan(&total); err != nil {
			return fmt.Errorf("commerce postgres: count branch quota rows: %w", err)
		}
		if total == 0 {
			return nil
		}

		limit, offset := f.Limit, f.Offset
		if limit <= 0 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}
		paged := append(append([]any{}, args...), limit, offset)
		listSQL := base + quotaRowSelect + where + fmt.Sprintf(`
			ORDER BY (COALESCE(c.used, 0) >= COALESCE(v.quota_limit, 0)) DESC,
			         c.last_order_at DESC NULLS LAST,
			         p.variant_id DESC, p.branch_id DESC
			LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)

		rows, err := tx.Query(txCtx, listSQL, paged...)
		if err != nil {
			return fmt.Errorf("commerce postgres: list branch quota rows: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row commerce.BranchQuotaRow
			if err := rows.Scan(
				&row.VariantID, &row.VariantName, &row.ProductID, &row.ProductName, &row.SKU,
				&row.QuotaLimit, &row.BranchID, &row.BranchName, &row.CustomerOrgID,
				&row.CustomerName, &row.Used, &row.OrderCount, &row.LastOrderAt,
				&row.ReleasedAt, &row.ReleaseCount, &row.ReleasedUnits,
			); err != nil {
				return fmt.Errorf("commerce postgres: scan branch quota row: %w", err)
			}
			rowsOut = append(rowsOut, &row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return rowsOut, total, nil
}

// ListQuotaVariantRows returns one page of "which of my items are restricted",
// with what has happened to each one's allowance across every branch.
func (r *Repository) ListQuotaVariantRows(
	ctx context.Context, vendorOrgID int64, f commerce.QuotaFilter,
) ([]*commerce.QuotaVariantRow, int, error) {
	if vendorOrgID <= 0 {
		return nil, 0, nil
	}
	base := replaceReleasing(quotaConsumptionCTE, "$2")
	args := []any{vendorOrgID, commerce.QuotaReleasingStatusStrings()}

	var clauses []string
	if f.VariantID > 0 {
		args = append(args, f.VariantID)
		clauses = append(clauses, fmt.Sprintf("v.id = $%d", len(args)))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+q+"%")
		n := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(v.name->>'ar' ILIKE $%d OR v.name->>'en' ILIKE $%d OR v.sku ILIKE $%d"+
				" OR pr.name->>'ar' ILIKE $%d OR pr.name->>'en' ILIKE $%d)", n, n, n, n, n))
	}
	where := " WHERE v.organization_id = $1 AND v.deleted_at IS NULL AND v.quota_limit IS NOT NULL"
	if len(clauses) > 0 {
		where += " AND " + strings.Join(clauses, " AND ")
	}

	var out []*commerce.QuotaVariantRow
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// The CTEs are carried into the count even though it does not read
		// them: they are where $2 is referenced, and a bind that supplies a
		// parameter the statement never mentions is rejected outright.
		// PostgreSQL does not evaluate an unreferenced, read-only CTE, so this
		// costs nothing at run time.
		countSQL := base + `
			SELECT count(*) FROM catalog.product_variants v
			LEFT JOIN catalog.products pr ON pr.id = v.product_id` + where
		if err := tx.QueryRow(txCtx, countSQL, args...).Scan(&total); err != nil {
			return fmt.Errorf("commerce postgres: count quota variants: %w", err)
		}
		if total == 0 {
			return nil
		}

		limit, offset := f.Limit, f.Offset
		if limit <= 0 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}
		paged := append(append([]any{}, args...), limit, offset)
		listSQL := base + `
			SELECT v.id,
			       COALESCE(NULLIF(v.name->>'ar', ''), NULLIF(v.name->>'en', ''), v.sku, ''),
			       COALESCE(v.product_id, 0),
			       COALESCE(NULLIF(pr.name->>'ar', ''), NULLIF(pr.name->>'en', ''), ''),
			       COALESCE(v.sku, ''),
			       COALESCE(v.quota_limit, 0),
			       COALESCE(agg.branch_count, 0)::int,
			       COALESCE(agg.exhausted_branches, 0)::int,
			       COALESCE(agg.total_used, 0)::int
			FROM catalog.product_variants v
			LEFT JOIN catalog.products pr ON pr.id = v.product_id
			LEFT JOIN LATERAL (
				SELECT COUNT(*)                                                    AS branch_count,
				       COUNT(*) FILTER (WHERE c.used >= COALESCE(v.quota_limit, 0)) AS exhausted_branches,
				       SUM(c.used)                                                 AS total_used
				FROM consumption c
				WHERE c.variant_id = v.id
			) agg ON true` + where + fmt.Sprintf(`
			ORDER BY COALESCE(agg.total_used, 0) DESC, v.id DESC
			LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)

		rows, err := tx.Query(txCtx, listSQL, paged...)
		if err != nil {
			return fmt.Errorf("commerce postgres: list quota variants: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row commerce.QuotaVariantRow
			if err := rows.Scan(
				&row.VariantID, &row.VariantName, &row.ProductID, &row.ProductName,
				&row.SKU, &row.QuotaLimit, &row.BranchCount, &row.ExhaustedBranches, &row.TotalUsed,
			); err != nil {
				return fmt.Errorf("commerce postgres: scan quota variant: %w", err)
			}
			out = append(out, &row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// QuotaSummaryForVendor computes the screen's stat cards in one round trip.
func (r *Repository) QuotaSummaryForVendor(ctx context.Context, vendorOrgID int64) (commerce.QuotaSummary, error) {
	var s commerce.QuotaSummary
	if vendorOrgID <= 0 {
		return s, nil
	}
	base := replaceReleasing(quotaConsumptionCTE, "$2")
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, base+`
			SELECT (SELECT count(*) FROM catalog.product_variants
			         WHERE organization_id = $1 AND deleted_at IS NULL AND quota_limit IS NOT NULL)::int,
			       (SELECT count(*) FROM consumption)::int,
			       (SELECT count(*) FROM consumption c
			          JOIN catalog.product_variants v ON v.id = c.variant_id
			         WHERE c.used >= COALESCE(v.quota_limit, 0))::int,
			       (SELECT COALESCE(SUM(used), 0) FROM consumption)::int,
			       (SELECT count(*) FROM released)::int;
		`, vendorOrgID, commerce.QuotaReleasingStatusStrings()).Scan(
			&s.VariantsWithQuota, &s.BranchesConsuming,
			&s.BranchesExhausted, &s.TotalUnitsUsed, &s.ReleasesMade,
		)
	})
	if err != nil {
		return commerce.QuotaSummary{}, fmt.Errorf("commerce postgres: quota summary: %w", err)
	}
	return s, nil
}
