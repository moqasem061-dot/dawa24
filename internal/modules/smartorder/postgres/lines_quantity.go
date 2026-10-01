package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// UpdateLineQuantity applies a buyer's edit to one line.
func (r *Repository) UpdateLineQuantity(ctx context.Context, orgID, lineID int64, qty float64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE smartorder.run_lines
			SET edited_qty = $3, effective_qty = $3
			WHERE id = $1 AND organization_id = $2;`, lineID, orgID, qty)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("smart_order_line")
		}
		return nil
	})
}
