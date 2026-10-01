package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// RecordShadowOutcomes persists shadow matching outcomes in batch (US13 T150-T151).
// Strictly scoped under the tenant context via WithTenant to respect FORCE RLS.
func (r *Repository) RecordShadowOutcomes(ctx context.Context, outcomes []matching.ShadowOutcome) error {
	if len(outcomes) == 0 {
		return nil
	}

	orgID := outcomes[0].OrganizationID
	return r.db.InTx(database.WithTenant(ctx, orgID), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.match_shadow_outcomes (
				run_id, organization_id, row_ref, shadow_policy_version, decision, created_at
			) VALUES ($1, $2, $3, $4, $5, now())
			ON CONFLICT (run_id, row_ref, shadow_policy_version)
			DO UPDATE SET decision = EXCLUDED.decision, created_at = now();
		`

		for _, out := range outcomes {
			decJSON, err := json.Marshal(out.Decision)
			if err != nil {
				return fmt.Errorf("marshal shadow decision for row %s: %w", out.RowRef, err)
			}
			_, err = tx.Exec(txCtx, query, out.RunID, out.OrganizationID, out.RowRef, out.ShadowPolicyVersion, decJSON)
			if err != nil {
				return fmt.Errorf("record shadow outcome for row %s: %w", out.RowRef, err)
			}
		}
		return nil
	})
}

// ListShadowOutcomes retrieves all shadow outcomes for a given run ID.
func (r *Repository) ListShadowOutcomes(ctx context.Context, runID int64) ([]matching.ShadowOutcome, error) {
	var results []matching.ShadowOutcome
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, run_id, organization_id, row_ref, shadow_policy_version, decision, created_at
			FROM catalog.match_shadow_outcomes
			WHERE run_id = $1
			ORDER BY id ASC;
		`
		rows, err := tx.Query(txCtx, query, runID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o matching.ShadowOutcome
			var decJSON []byte
			if err := rows.Scan(&o.ID, &o.RunID, &o.OrganizationID, &o.RowRef, &o.ShadowPolicyVersion, &decJSON, &o.CreatedAt); err != nil {
				return err
			}
			if len(decJSON) > 0 {
				_ = json.Unmarshal(decJSON, &o.Decision)
			}
			results = append(results, o)
		}
		return nil
	})
	return results, err
}
