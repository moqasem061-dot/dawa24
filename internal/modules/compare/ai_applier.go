package compare

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// CompareAIApplier applies AI decisions to compare.file_rows with revision compare-and-set (T127).
type CompareAIApplier struct {
	db *database.DB
}

// NewCompareAIApplier constructs a CompareAIApplier.
func NewCompareAIApplier(db *database.DB) *CompareAIApplier {
	return &CompareAIApplier{db: db}
}

// ApplyAI applies an AI decision with a revision compare-and-set.
// Returns false if the row was modified (obsolete) or human-locked.
func (a *CompareAIApplier) ApplyAI(ctx context.Context, ref productmatch.RowRef, expectedRev int, runID int64, d matchflow.RowDecision) (bool, error) {
	data, err := d.MarshalJSON()
	if err != nil {
		return false, err
	}
	cols := matchflow.Columns(d)

	var applied bool
	err = a.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		cmd, err := tx.Exec(txCtx, `
			UPDATE compare.file_rows
			SET match_revision     = match_revision + 1,
			    match_state        = $1,
			    match_basis        = $2,
			    match_granularity  = $3,
			    ai_state           = $4,
			    match_run_id       = $5,
			    match_decision     = $6
			WHERE id = $7
			  AND match_revision = $8
			  AND human_locked = false
		`, cols.MatchState, cols.MatchBasis, cols.MatchGranularity,
			cols.AIState, runID, data, ref.ID, expectedRev)
		if err != nil {
			return err
		}
		applied = cmd.RowsAffected() > 0
		return nil
	})
	return applied, err
}
