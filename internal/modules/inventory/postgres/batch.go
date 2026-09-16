package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// AvailableQuantities totals the sellable stock of many variants across all
// warehouses in one grouped query. Storefront offer rendering used to ask per
// variant; a catalog page of 100 products × 3 variants meant 300 round trips.
//
// AsSystem, like AvailableQuantity: asking whether one may buy another
// tenant's stock crosses the tenant boundary by definition.
func (r *Repository) AvailableQuantities(ctx context.Context, variantIDs []int64) (map[int64]int, error) {
	out := make(map[int64]int, len(variantIDs))
	if len(variantIDs) == 0 {
		return out, nil
	}
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const q = `
			SELECT s.product_variant_id, COALESCE(SUM(s.quantity), 0)
			FROM inventory.stocks s
			WHERE s.product_variant_id = ANY($1)
			  AND s.deleted_at IS NULL
			GROUP BY s.product_variant_id;
		`
		rows, err := tx.Query(txCtx, q, variantIDs)
		if err != nil {
			return fmt.Errorf("inventory postgres: available quantities: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var variantID int64
			var qty int
			if err := rows.Scan(&variantID, &qty); err != nil {
				return err
			}
			if qty < 0 {
				// Match AvailableQuantity: an inconsistent negative ledger is
				// reported as no availability.
				qty = 0
			}
			out[variantID] = qty
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
