package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The quota read, for a page rather than for a line.
//
// BranchQuotaUsed answers one variant at a time, which is right for a purchase:
// there is one line and the answer gates it. It is wrong for a listing. The
// buying catalogue evaluates up to ninety-six offers at once, and asking per
// offer turned one page view into ninety-six sequential round trips against a
// database the application reaches over the public internet — sixty-odd
// milliseconds each, which is six seconds of a page load spent asking the same
// question with a different id.
//
// This is the same SUM over the same predicate, grouped. One statement answers
// the page, and the predicate is quotaCountsSQL, so a number here cannot differ
// from the number the gate enforces.

// BranchQuotaUsedBatch sums one branch's committed purchases of many variants.
//
// Variants with no committed purchases are absent from the result rather than
// present with a zero: the caller already knows which ids it asked about, and a
// missing key is the same answer as zero without the row.
func (r *Repository) BranchQuotaUsedBatch(
	ctx context.Context, variantIDs []int64, branchID int64,
) (map[int64]int, error) {
	out := make(map[int64]int, len(variantIDs))
	if len(variantIDs) == 0 || branchID <= 0 {
		return out, nil
	}

	query := `
		SELECT ol.product_variant_id, COALESCE(SUM(ol.quantity), 0)
		FROM commerce.order_lines ol
		JOIN commerce.orders o ON o.id = ol.order_id` + quotaJoinSQL + `
		WHERE ol.product_variant_id = ANY($1)
		  AND o.branch_id = $2
		  AND ` + quotaCountsSQL + `
		GROUP BY ol.product_variant_id`
	query = replaceReleasing(query, "$3")

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, query,
			variantIDs, branchID, commerce.QuotaReleasingStatusStrings())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var variantID int64
			var used int
			if err := rows.Scan(&variantID, &used); err != nil {
				return err
			}
			if used < 0 {
				used = 0
			}
			out[variantID] = used
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("commerce postgres: branch quota used batch: %w", err)
	}
	return out, nil
}
