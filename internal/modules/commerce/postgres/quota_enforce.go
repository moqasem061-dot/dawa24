package postgres

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The quota's last line of defence: the check that runs inside the order's own
// transaction.
//
// commerce.CheckAvailability already refuses an over-quota line at the cart, at
// the catalogue and again at checkout, and that is where a pharmacy gets a
// message it can act on. None of those checks is atomic with the write, though:
// two branches — or the same branch in two tabs — can pass the gate a
// microsecond apart and both arrive here with the last three units. So the rule
// is applied once more against a lock held for the life of the transaction that
// inserts the lines, and a refusal here is a conflict rather than a validation
// error, because the caller did nothing wrong.

// quotaDemand is one variant's total demand in an order being written.
type quotaDemand struct {
	variantID int64
	quantity  int
}

// enforceOrderQuotas refuses the write if any line would take a branch past the
// supplier's per-branch cap.
//
// excludeOrderID discounts an order's own existing lines, so an edit that
// raises a line from 2 to 3 is measured as 3 and not as 5. It is zero for a new
// order, which has no lines of its own yet.
func enforceOrderQuotas(
	ctx context.Context, tx pgx.Tx, branchID int64, demands []quotaDemand, excludeOrderID int64,
) error {
	if len(demands) == 0 {
		return nil
	}

	// Sum first: a variant can appear on more than one line of the same order.
	totals := make(map[int64]int, len(demands))
	for _, d := range demands {
		if d.variantID > 0 && d.quantity > 0 {
			totals[d.variantID] += d.quantity
		}
	}
	if len(totals) == 0 {
		return nil
	}

	// Locks are taken in a fixed order. Two orders touching the same pair of
	// variants in opposite orders would otherwise deadlock, and a deadlock at
	// checkout looks to the pharmacy like the site falling over.
	variantIDs := make([]int64, 0, len(totals))
	for id := range totals {
		variantIDs = append(variantIDs, id)
	}
	sort.Slice(variantIDs, func(i, j int) bool { return variantIDs[i] < variantIDs[j] })

	for _, variantID := range variantIDs {
		limit, err := variantQuotaLimitTx(ctx, tx, variantID)
		if err != nil {
			return err
		}
		if limit <= 0 {
			continue // no quota on this item
		}

		// A quota is per *branch*. An order that names no receiving branch
		// cannot be attributed to one, so it cannot be allowed to consume a
		// restricted item: it would be an untracked purchase against a cap
		// whose whole purpose is to be tracked.
		if branchID <= 0 {
			return apperr.Validation("checkout.quota_branch_required",
				i18n.TDefault("quota.branch_required"),
				map[string]string{"branch": "required"})
		}

		if err := lockQuotaPairing(ctx, tx, variantID, branchID); err != nil {
			return err
		}
		used, err := branchQuotaUsedTx(ctx, tx, variantID, branchID, excludeOrderID)
		if err != nil {
			return err
		}
		usage := commerce.BranchQuotaUsage{
			VariantID: variantID, BranchID: branchID, Limit: limit, Used: used,
		}
		remaining := usage.Remaining()
		if totals[variantID] > remaining {
			return quotaConflict(ctx, tx, variantID, limit, remaining)
		}
	}
	return nil
}

// quotaConflict names the item in the refusal. A pharmacy with eleven lines in
// its basket told only "quota exceeded" has to guess which one to change.
func quotaConflict(ctx context.Context, tx pgx.Tx, variantID int64, limit, remaining int) error {
	name := variantDisplayName(ctx, tx, variantID)
	if remaining <= 0 {
		return apperr.Conflict("checkout.quota_exhausted",
			fmt.Sprintf(i18n.TDefault("quota.exhausted_named"), name, limit))
	}
	return apperr.Conflict("checkout.quota_exceeded",
		fmt.Sprintf(i18n.TDefault("quota.exceeded_named"), name, remaining, limit))
}

// variantDisplayName reads a variant's name for a message. A failure here is
// not worth failing the refusal over, so it degrades to the SKU and then to a
// generic noun.
func variantDisplayName(ctx context.Context, tx pgx.Tx, variantID int64) string {
	var name string
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(
		         NULLIF(pr.name->>'ar', ''), NULLIF(pr.name->>'en', ''),
		         NULLIF(v.name->>'ar', ''),  NULLIF(v.name->>'en', ''),
		         NULLIF(v.sku, ''), '')
		FROM catalog.product_variants v
		LEFT JOIN catalog.products pr ON pr.id = v.product_id
		WHERE v.id = $1;
	`, variantID).Scan(&name)
	if err != nil || name == "" {
		return i18n.TDefault("quota.unnamed_item")
	}
	return name
}

// orderQuotaDemandsTx reads an order's current lines as quota demands.
//
// The order editor uses it rather than its own edit list, because a line the
// customer did not touch still consumes quota and because the list carries line
// ids rather than variant ids.
func orderQuotaDemandsTx(ctx context.Context, tx pgx.Tx, orderID int64) ([]quotaDemand, error) {
	rows, err := tx.Query(ctx, `
		SELECT product_variant_id, quantity
		FROM commerce.order_lines
		WHERE order_id = $1 AND product_variant_id IS NOT NULL;
	`, orderID)
	if err != nil {
		return nil, fmt.Errorf("commerce postgres: read order quota demands: %w", err)
	}
	defer rows.Close()

	var out []quotaDemand
	for rows.Next() {
		var d quotaDemand
		if err := rows.Scan(&d.variantID, &d.quantity); err != nil {
			return nil, fmt.Errorf("commerce postgres: scan order quota demand: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// orderLineDemands projects the lines being written onto what the quota rule
// needs from them.
func orderLineDemands(lines []*commerce.OrderLine) []quotaDemand {
	out := make([]quotaDemand, 0, len(lines))
	for _, l := range lines {
		if l == nil || l.ProductVariantID == nil || *l.ProductVariantID <= 0 {
			continue
		}
		out = append(out, quotaDemand{variantID: *l.ProductVariantID, quantity: l.Quantity})
	}
	return out
}
