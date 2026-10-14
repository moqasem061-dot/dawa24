package ingest

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// CatalogAIApplier applies AI decisions to ingest.catalog_import_rows with revision compare-and-set (T127).
type CatalogAIApplier struct {
	db *database.DB
}

// NewCatalogAIApplier constructs a CatalogAIApplier.
func NewCatalogAIApplier(db *database.DB) *CatalogAIApplier {
	return &CatalogAIApplier{db: db}
}

// ApplyAI applies an AI decision with a revision compare-and-set.
// Returns false if the row was modified (obsolete) or human-locked.
func (a *CatalogAIApplier) ApplyAI(ctx context.Context, ref productmatch.RowRef, expectedRev int, runID int64, d matchflow.RowDecision) (bool, error) {
	data, err := d.MarshalJSON()
	if err != nil {
		return false, err
	}
	cols := matchflow.Columns(d)

	var applied bool
	err = a.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		cmd, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_import_rows
			SET match_revision     = match_revision + 1,
			    match_state        = $1,
			    match_basis        = $2,
			    match_granularity  = $3,
			    product_id         = $4,
			    variant_id         = CASE WHEN product_id IS NOT DISTINCT FROM $4 THEN variant_id ELSE NULL END,
			    match_level        = $5,
			    ai_state           = $6,
			    match_run_id       = $7,
			    match_decision     = $8,
			    match_score        = $11,
			    candidates         = COALESCE((
			        SELECT jsonb_agg(jsonb_build_object(
			            'product_id', p.id, 'name', COALESCE(p.name->>'ar', p.name->>'en', ''),
			            'scientific', COALESCE(p.scientific_name, ''),
			            'dosage_form', COALESCE(p.dosage_form, ''),
			            'concentration', COALESCE(p.concentration, ''),
			            'manufacturer', COALESCE(p.manufacturing_companies, ''),
			            'score', COALESCE(c.entry->'score', '0'::jsonb)
			        ) ORDER BY c.position)
			        FROM jsonb_array_elements(COALESCE($8::jsonb->'candidates', '[]'::jsonb))
			            WITH ORDINALITY AS c(entry, position)
			        JOIN catalog.products p ON p.id = (c.entry->>'product_id')::bigint
			    ), '[]'::jsonb)
			WHERE id = $9
			  AND match_revision = $10
			  AND human_locked = false
		`, cols.MatchState, cols.MatchBasis, cols.MatchGranularity,
			cols.MatchedProductID, cols.MatchLevel, cols.AIState,
			runID, data, ref.ID, expectedRev, decisionMatchScore(d))
		if err != nil {
			return err
		}
		applied = cmd.RowsAffected() > 0
		return nil
	})
	return applied, err
}
