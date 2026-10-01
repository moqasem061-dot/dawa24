package catalog

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// CatalogImportAIApplier applies AI decisions to catalog.import_staging_rows with revision compare-and-set (T127).
type CatalogImportAIApplier struct {
	db *database.DB
}

// NewCatalogImportAIApplier constructs a CatalogImportAIApplier.
func NewCatalogImportAIApplier(db *database.DB) *CatalogImportAIApplier {
	return &CatalogImportAIApplier{db: db}
}

// ApplyAI applies an AI decision with a revision compare-and-set.
// Returns false if the row was modified (obsolete) or human-locked.
func (a *CatalogImportAIApplier) ApplyAI(ctx context.Context, ref productmatch.RowRef, expectedRev int, runID int64, d matchflow.RowDecision) (bool, error) {
	data, err := d.MarshalJSON()
	if err != nil {
		return false, err
	}
	cols := matchflow.Columns(d)
	proposal := string(ClassifyProposal(d, 0))

	var applied bool
	err = a.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		cmd, err := tx.Exec(txCtx, `
			UPDATE catalog.import_staging_rows
			SET match_revision     = match_revision + 1,
			    match_state        = $1,
			    match_basis        = $2,
			    match_granularity  = $3,
			    ai_state           = $4,
			    match_run_id       = $5,
			    match_decision     = $6,
			    proposal           = $7
			WHERE id = $8
			  AND match_revision = $9
			  AND human_locked = false
		`, cols.MatchState, cols.MatchBasis, cols.MatchGranularity,
			cols.AIState, runID, data, proposal, ref.ID, expectedRev)
		if err != nil {
			return err
		}
		applied = cmd.RowsAffected() > 0
		return nil
	})
	return applied, err
}
