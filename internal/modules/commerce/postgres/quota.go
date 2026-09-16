package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The per-branch quota, in SQL.
//
// Everything here rests on one expression — quotaCountsSQL — which decides
// whether an order line still holds quota. It is written once and reused by the
// gate, by the atomic check inside checkout, by the order editor and by the
// supplier's report, because a report that disagrees with the gate is worse
// than no report: the supplier reads "3 of 10 used" and the pharmacy is told it
// has none left.

// quotaCountsSQL is the predicate for "this order still holds quota".
//
// Two conditions, and they are both about the order rather than the line:
//   - it has not been undone (cancelled / failed / returned / refunded), and it
//     has not been soft-deleted;
//   - it was placed after the supplier last released this branch, if ever.
//
// The release comparison uses >, not >=: an order placed in the same
// microsecond as the release is on the far side of it. That is a distinction
// without a difference in practice and the strict form is the one that makes
// "released" mean "everything up to now is forgiven".
const quotaCountsSQL = `
	o.deleted_at IS NULL
	AND o.status <> ALL ($RELEASING$)
	AND o.created_at > COALESCE(rel.released_at, '-infinity'::timestamptz)`

// quotaJoinSQL attaches the release marker for the (variant, branch) pairing a
// line belongs to. LEFT, because most pairings have never been released.
const quotaJoinSQL = `
	LEFT JOIN commerce.variant_branch_quota_releases rel
	       ON rel.product_variant_id = ol.product_variant_id
	      AND rel.branch_id = o.branch_id`

// quotaLockKey serialises two checkouts racing for the last units of one
// branch's allowance.
//
// A transaction-scoped advisory lock rather than a row lock, because the thing
// being protected — the SUM over order lines — is not a row: there may be no
// release row to lock and no prior order to lock either, which is exactly the
// case where two first-time buyers would both read zero and both be allowed
// through. The lock is released when the transaction ends, committed or not.
func quotaLockKey(variantID, branchID int64) string {
	return fmt.Sprintf("dawa24:variant-branch-quota:%d:%d", variantID, branchID)
}

func lockQuotaPairing(ctx context.Context, tx pgx.Tx, variantID, branchID int64) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0));`,
		quotaLockKey(variantID, branchID))
	if err != nil {
		return fmt.Errorf("commerce postgres: lock quota pairing: %w", err)
	}
	return nil
}

// branchQuotaUsedTx sums what one branch has committed to buying of one
// variant, inside a caller's transaction.
//
// excludeOrderID discounts one order's own lines. The order editor passes the
// order being edited so that raising a line from 2 to 3 is measured as 3
// against the cap rather than as 5 — the edit replaces the commitment, it does
// not add to it.
func branchQuotaUsedTx(
	ctx context.Context, tx pgx.Tx, variantID, branchID, excludeOrderID int64,
) (int, error) {
	if variantID <= 0 || branchID <= 0 {
		return 0, nil
	}
	query := `
		SELECT COALESCE(SUM(ol.quantity), 0)
		FROM commerce.order_lines ol
		JOIN commerce.orders o ON o.id = ol.order_id` + quotaJoinSQL + `
		WHERE ol.product_variant_id = $1
		  AND o.branch_id = $2
		  AND ($3::bigint = 0 OR o.id <> $3)
		  AND ` + quotaCountsSQL
	query = replaceReleasing(query, "$4")

	var used int
	if err := tx.QueryRow(ctx, query,
		variantID, branchID, excludeOrderID, commerce.QuotaReleasingStatusStrings(),
	).Scan(&used); err != nil {
		return 0, fmt.Errorf("commerce postgres: branch quota used: %w", err)
	}
	if used < 0 {
		used = 0
	}
	return used, nil
}

// replaceReleasing substitutes the placeholder in quotaCountsSQL for the
// parameter number the calling query happens to have reached. The predicate is
// shared by five queries with different argument counts, and threading a
// formatter through each of them was worse than one named placeholder.
func replaceReleasing(query, param string) string {
	return strings.ReplaceAll(query, "$RELEASING$", param)
}

// BranchQuotaUsed sums one branch's committed purchases of one variant.
//
// AsSystem: the caller is the buying pharmacy asking whether it may buy from a
// supplier, so it is by definition summing rows that belong to orders it may
// not otherwise read in bulk. Only an integer crosses the boundary, and only to
// answer "may I buy this".
func (r *Repository) BranchQuotaUsed(ctx context.Context, variantID, branchID, excludeOrderID int64) (int, error) {
	var used int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		n, err := branchQuotaUsedTx(txCtx, tx, variantID, branchID, excludeOrderID)
		used = n
		return err
	})
	if err != nil {
		return 0, err
	}
	return used, nil
}

// variantQuotaLimitTx reads a variant's cap, 0 meaning unlimited.
//
// It reads catalog.product_variants directly rather than through the catalog
// module, the same way this file's neighbours already read min_order_qty and
// inventory.stocks: the check has to happen inside the order's transaction, and
// a service call cannot join that transaction.
func variantQuotaLimitTx(ctx context.Context, tx pgx.Tx, variantID int64) (int, error) {
	if variantID <= 0 {
		return 0, nil
	}
	var limit *int
	err := tx.QueryRow(ctx, `
		SELECT quota_limit
		FROM catalog.product_variants
		WHERE id = $1 AND deleted_at IS NULL;
	`, variantID).Scan(&limit)
	if err != nil {
		if database.IsNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("commerce postgres: variant quota limit: %w", err)
	}
	if limit == nil || *limit <= 0 {
		return 0, nil
	}
	return *limit, nil
}
