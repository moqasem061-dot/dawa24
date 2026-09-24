package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
)

// SetDefaultQuantity applies a quantity to all (or selected) lines in a run, and updates selections.
func (r *Repository) SetDefaultQuantity(ctx context.Context, orgID, runID int64, lineIDs []int64, qty float64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Update lines in run_lines, capping at available supplier stock if selected
		var err error
		if len(lineIDs) > 0 {
			_, err = tx.Exec(txCtx, `
				UPDATE smartorder.run_lines l
				SET edited_qty = LEAST($1::numeric, COALESCE((
					SELECT NULLIF(c.stock_qty, 0)
					FROM smartorder.line_selections s
					JOIN smartorder.line_candidates c ON c.id = s.candidate_id
					WHERE s.line_id = l.id AND s.organization_id = $3
				), $1::numeric)),
				    effective_qty = LEAST($1::numeric, COALESCE((
					SELECT NULLIF(c.stock_qty, 0)
					FROM smartorder.line_selections s
					JOIN smartorder.line_candidates c ON c.id = s.candidate_id
					WHERE s.line_id = l.id AND s.organization_id = $3
				), $1::numeric))
				WHERE run_id = $2 AND organization_id = $3 AND id = ANY($4);`,
				qty, runID, orgID, lineIDs)
		} else {
			_, err = tx.Exec(txCtx, `
				UPDATE smartorder.run_lines l
				SET edited_qty = LEAST($1::numeric, COALESCE((
					SELECT NULLIF(c.stock_qty, 0)
					FROM smartorder.line_selections s
					JOIN smartorder.line_candidates c ON c.id = s.candidate_id
					WHERE s.line_id = l.id AND s.organization_id = $3
				), $1::numeric)),
				    effective_qty = LEAST($1::numeric, COALESCE((
					SELECT NULLIF(c.stock_qty, 0)
					FROM smartorder.line_selections s
					JOIN smartorder.line_candidates c ON c.id = s.candidate_id
					WHERE s.line_id = l.id AND s.organization_id = $3
				), $1::numeric))
				WHERE run_id = $2 AND organization_id = $3 AND outcome = 'ordered';`,
				qty, runID, orgID)
		}
		if err != nil {
			return err
		}

		// 2. Update selections line_net
		if qty == 0 {
			if len(lineIDs) > 0 {
				_, err = tx.Exec(txCtx, `
					DELETE FROM smartorder.line_selections
					WHERE organization_id = $1 AND line_id = ANY($2);`,
					orgID, lineIDs)
			} else {
				_, err = tx.Exec(txCtx, `
					DELETE FROM smartorder.line_selections s
					USING smartorder.run_lines l
					WHERE s.line_id = l.id AND l.run_id = $1 AND s.organization_id = $2;`,
					runID, orgID)
			}
			return err
		}

		// Recalculate line_net for updated selections using candidate net_unit_price and line effective_qty
		_, err = tx.Exec(txCtx, `
			UPDATE smartorder.line_selections s
			SET line_net = ROUND(c.net_unit_price * l.effective_qty, 2),
			    updated_at = now()
			FROM smartorder.line_candidates c, smartorder.run_lines l
			WHERE s.candidate_id = c.id
			  AND s.line_id = l.id
			  AND l.run_id = $1
			  AND s.organization_id = $2
			  AND ($3::bigint[] IS NULL OR cardinality($3::bigint[]) = 0 OR l.id = ANY($3));
		`, runID, orgID, lineIDs)
		return err
	})
}

// BlockedCounts tallies the lines that will not be ordered, by reason.
func (r *Repository) BlockedCounts(ctx context.Context, runID int64) (smartorder.BlockedCounts, error) {
	var bc smartorder.BlockedCounts
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT
				COUNT(*) FILTER (WHERE outcome = 'unmatched'),
				COUNT(*) FILTER (WHERE outcome = 'no_supplier'),
				COUNT(*) FILTER (WHERE outcome = 'coverage_blocked'),
				COUNT(*) FILTER (WHERE outcome = 'institutional_blocked'),
				COUNT(*) FILTER (WHERE outcome = 'out_of_stock'),
				COUNT(*) FILTER (WHERE outcome = 'below_min_qty'),
				COUNT(*) FILTER (WHERE outcome = 'zero_qty'),
				COUNT(*) FILTER (WHERE outcome = 'removed'),
				COUNT(*) FILTER (WHERE outcome = 'quota_blocked')
			FROM smartorder.run_lines
			WHERE run_id = $1;`, runID).Scan(
			&bc.Unmatched, &bc.NoSupplier, &bc.CoverageBlocked, &bc.InstitutionalBlocked,
			&bc.OutOfStock, &bc.BelowMinQty, &bc.ZeroQty, &bc.Removed, &bc.QuotaBlocked)
	})
	return bc, err
}

// FilterCounts returns exact row counts for each independent smart order filter tab.
func (r *Repository) FilterCounts(ctx context.Context, runID int64) (smartorder.FilterCounts, error) {
	var fc smartorder.FilterCounts
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT
				COUNT(*),
				COUNT(*) FILTER (WHERE matched_product_id IS NULL OR matched_product_id = 0),
				COUNT(*) FILTER (WHERE matched_product_id IS NOT NULL AND matched_product_id > 0),
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = l.id AND c.eligible)),
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = l.id AND c.eligible AND c.stock_qty > 0)),
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = l.id AND c.eligible AND (c.ineligible_reason IS NULL OR c.ineligible_reason != 'coverage'))),
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = l.id AND c.eligible AND c.net_unit_price > 0)),
				COUNT(*) FILTER (WHERE outcome = 'ordered')
			FROM smartorder.run_lines l
			WHERE l.run_id = $1;
		`
		return tx.QueryRow(txCtx, query, runID).Scan(
			&fc.Total,
			&fc.Unmatched,
			&fc.MatchedProduct,
			&fc.MatchedSupplier,
			&fc.AvailableStock,
			&fc.CoveredBranch,
			&fc.PriceAvailable,
			&fc.ReadyToOrder,
		)
	})
	return fc, err
}
