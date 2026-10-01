package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// LookupDecisions reads the adjudication cache for a batch of keys.
// When scoped to an organization, it queries the organization's own decisions AND
// platform-wide decisions (unless the organization has opted out in preferences).
// If a key collision occurs, the organization's own decision always wins.
func (r *Repository) LookupDecisions(ctx context.Context, keys []string) (map[string]smartorder.CachedDecision, error) {
	if len(keys) == 0 {
		return map[string]smartorder.CachedDecision{}, nil
	}
	out := make(map[string]smartorder.CachedDecision, len(keys))

	var orgID int64
	if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
		orgID = actor.OrganizationID
	} else if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
		orgID = tid
	}

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if !isDecisionMemoryEnabled(txCtx, tx) {
			return nil // Global kill-switch is OFF
		}

		var rows pgx.Rows
		var err error
		if orgID > 0 {
			usePlatform := true
			var prefVal bool
			if pErr := tx.QueryRow(txCtx, `SELECT use_platform_memory FROM catalog.decision_memory_preferences WHERE organization_id = $1`, orgID).Scan(&prefVal); pErr == nil {
				usePlatform = prefVal
			}

			if usePlatform {
				rows, err = tx.Query(txCtx, `
					UPDATE catalog.match_decisions
					SET hit_count = hit_count + 1, last_used_at = now()
					WHERE (organization_id = $1 OR scope = 'platform') AND decision_key = ANY($2::text[])
					RETURNING decision_key, norm_name, chosen_product_id, confidence, reason, prompt_version, scope, source, organization_id;`,
					orgID, keys)
			} else {
				rows, err = tx.Query(txCtx, `
					UPDATE catalog.match_decisions
					SET hit_count = hit_count + 1, last_used_at = now()
					WHERE organization_id = $1 AND decision_key = ANY($2::text[])
					RETURNING decision_key, norm_name, chosen_product_id, confidence, reason, prompt_version, scope, source, organization_id;`,
					orgID, keys)
			}
		} else {
			rows, err = tx.Query(txCtx, `
				UPDATE catalog.match_decisions
				SET hit_count = hit_count + 1, last_used_at = now()
				WHERE (scope = 'platform' OR organization_id IS NULL) AND decision_key = ANY($1::text[])
				RETURNING decision_key, norm_name, chosen_product_id, confidence, reason, prompt_version, scope, source, organization_id;`,
				keys)
		}
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d smartorder.CachedDecision
			var rowOrgID *int64
			if err := rows.Scan(&d.Key, &d.NormName, &d.ChosenProductID,
				&d.Confidence, &d.Reason, &d.PromptVersion, &d.Scope, &d.Source, &rowOrgID); err != nil {
				return err
			}
			existing, exists := out[d.Key]
			if !exists {
				out[d.Key] = d
			} else if existing.Scope == "platform" && (d.Scope == "org" || (rowOrgID != nil && *rowOrgID == orgID)) {
				// Org decision wins over platform decision on collision
				out[d.Key] = d
			}
		}
		return rows.Err()
	})
	return out, err
}

// SaveDecisions writes adjudication results to the cache.
// Legacy catalog.match_decisions is read-only (US8 T114); writes route through matching.Service.
func (r *Repository) SaveDecisions(ctx context.Context, decisions []smartorder.CachedDecision) error {
	return nil
}

func isDecisionMemoryEnabled(ctx context.Context, tx pgx.Tx) bool {
	var val any
	err := tx.QueryRow(ctx, `SELECT value FROM platform_admin.system_settings WHERE key = 'decision_memory_enabled' LIMIT 1;`).Scan(&val)
	if err != nil || val == nil {
		return true // default enabled
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1" || v == "yes"
	case []byte:
		s := string(v)
		return strings.Contains(s, "true") || strings.Contains(s, "1")
	default:
		return true
	}
}
