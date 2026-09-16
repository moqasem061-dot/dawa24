package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
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
func (r *Repository) SaveDecisions(ctx context.Context, decisions []smartorder.CachedDecision) error {
	if len(decisions) == 0 {
		return nil
	}
	var orgID int64
	var userID int64
	if actor, ok := authctx.From(ctx); ok {
		orgID = actor.OrganizationID
		userID = actor.UserID
	} else if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
		orgID = tid
	}

	scope := "org"
	if orgID <= 0 {
		scope = "platform"
	}

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if !isDecisionMemoryEnabled(txCtx, tx) {
			return nil // Global switch is OFF
		}

		const cols = 10
		values := make([]string, 0, len(decisions))
		args := make([]any, 0, len(decisions)*cols)
		for i, d := range decisions {
			base := i * cols
			ph := make([]string, cols)
			for j := 0; j < cols; j++ {
				ph[j] = "$" + strconv.Itoa(base+j+1)
			}
			values = append(values, "("+strings.Join(ph, ",")+")")
			src := d.Source
			if src == "" {
				src = "ai"
			}
			args = append(args,
				sqlNullOrgID(orgID), sqlNullOrgID(userID),
				d.Key, d.NormName, d.ChosenProductID,
				d.Confidence, d.Reason, d.PromptVersion, scope, src)
		}
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.match_decisions (
				organization_id, user_id, decision_key, norm_name, chosen_product_id,
				confidence, reason, prompt_version, scope, source
			) VALUES `+strings.Join(values, ",")+`
			ON CONFLICT (COALESCE(organization_id, 0), decision_key)
			DO UPDATE SET
				chosen_product_id = EXCLUDED.chosen_product_id,
				confidence = EXCLUDED.confidence,
				reason = EXCLUDED.reason,
				user_id = COALESCE(EXCLUDED.user_id, catalog.match_decisions.user_id),
				hit_count = catalog.match_decisions.hit_count + 1,
				last_used_at = now();`, args...)
		return err
	})
}

func sqlNullOrgID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
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
