package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// LookupOrg retrieves active knowledge for questions scoped to the caller's tenant (RLS-guarded).
func (r *Repository) LookupOrg(ctx context.Context, sigs []string) ([]matching.Knowledge, error) {
	if len(sigs) == 0 {
		return nil, nil
	}

	var results []matching.Knowledge
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, organization_id, source_namespace, question_signature, kind,
			       target_product_id, target_granularity, target_fact_fingerprint, candidate_set_fingerprint,
			       policy_version, normalizer_version, ai_config_id, prompt_version, display_text,
			       status, legacy, created_by, created_at, revoked_by, revoked_at, last_used_at
			FROM catalog.match_knowledge
			WHERE question_signature = ANY($1)
			  AND status = 'active';
		`
		rows, err := tx.Query(txCtx, query, sigs)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			k, scanErr := scanKnowledge(rows)
			if scanErr != nil {
				return scanErr
			}
			results = append(results, k)
		}
		return rows.Err()
	})
	return results, err
}

// LookupPlatform retrieves global active platform knowledge (org_id IS NULL) under database.AsSystem.
// This is the ONLY place using database.AsSystem for knowledge, and it strictly respects opt-out.
func (r *Repository) LookupPlatform(ctx context.Context, sigs []string, orgID int64) ([]matching.Knowledge, error) {
	if len(sigs) == 0 {
		return nil, nil
	}

	var results []matching.Knowledge
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Check if the tenant opted out of platform memory (FR-054)
		if orgID > 0 {
			var usePlatform bool
			queryPref := `SELECT COALESCE((SELECT use_platform_memory FROM catalog.decision_memory_preferences WHERE organization_id = $1), true)`
			if err := tx.QueryRow(txCtx, queryPref, orgID).Scan(&usePlatform); err != nil {
				return err
			}
			if !usePlatform {
				return nil
			}
		}

		// 2. Fetch platform knowledge
		query := `
			SELECT id, organization_id, source_namespace, question_signature, kind,
			       target_product_id, target_granularity, target_fact_fingerprint, candidate_set_fingerprint,
			       policy_version, normalizer_version, ai_config_id, prompt_version, display_text,
			       status, legacy, created_by, created_at, revoked_by, revoked_at, last_used_at
			FROM catalog.match_knowledge
			WHERE organization_id IS NULL
			  AND question_signature = ANY($1)
			  AND status = 'active';
		`
		rows, err := tx.Query(txCtx, query, sigs)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			k, scanErr := scanKnowledge(rows)
			if scanErr != nil {
				return scanErr
			}
			results = append(results, k)
		}
		return rows.Err()
	})
	return results, err
}

// Upsert inserts or updates a match knowledge record on conflict.
func (r *Repository) Upsert(ctx context.Context, k matching.Knowledge) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.match_knowledge (
				organization_id, source_namespace, question_signature, kind,
				target_product_id, target_granularity, target_fact_fingerprint, candidate_set_fingerprint,
				policy_version, normalizer_version, ai_config_id, prompt_version, display_text,
				status, legacy, created_by, created_at, last_used_at
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7, $8,
				$9, $10, $11, $12, $13,
				$14, $15, $16, now(), now()
			)
			ON CONFLICT (COALESCE(organization_id, 0), source_namespace, question_signature, kind)
				WHERE status IN ('active', 'pending_revalidation')
			DO UPDATE SET
				target_product_id = EXCLUDED.target_product_id,
				target_granularity = EXCLUDED.target_granularity,
				target_fact_fingerprint = EXCLUDED.target_fact_fingerprint,
				candidate_set_fingerprint = EXCLUDED.candidate_set_fingerprint,
				policy_version = EXCLUDED.policy_version,
				normalizer_version = EXCLUDED.normalizer_version,
				ai_config_id = EXCLUDED.ai_config_id,
				prompt_version = EXCLUDED.prompt_version,
				display_text = EXCLUDED.display_text,
				status = EXCLUDED.status,
				last_used_at = now();
		`
		_, err := tx.Exec(txCtx, query,
			k.OrganizationID, k.SourceNamespace, k.QuestionSignature, k.Kind,
			k.TargetProductID, k.TargetGranularity, k.TargetFactFingerprint, k.CandidateSetFingerprint,
			k.PolicyVersion, k.NormalizerVersion, k.AIConfigID, k.PromptVersion, k.DisplayText,
			k.Status, k.Legacy, k.CreatedBy,
		)
		return err
	})
}

// Revoke marks a knowledge entry as revoked.
func (r *Repository) Revoke(ctx context.Context, id int64, revokedBy string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.match_knowledge
			SET status = 'revoked', revoked_at = now(), revoked_by = $2
			WHERE id = $1;
		`
		_, err := tx.Exec(txCtx, query, id, revokedBy)
		return err
	})
}

// MarkStale flags knowledge entries as stale when target catalog facts change.
func (r *Repository) MarkStale(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.match_knowledge SET status = 'stale' WHERE id = ANY($1);`
		_, err := tx.Exec(txCtx, query, ids)
		return err
	})
}

// List queries match knowledge records according to filters and pagination.
func (r *Repository) List(ctx context.Context, f matching.KnowledgeFilter) (matching.Page[matching.Knowledge], error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	var page matching.Page[matching.Knowledge]

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var conditions []string
		var args []any
		argIdx := 1

		if f.Namespace != "" {
			conditions = append(conditions, fmt.Sprintf("source_namespace = $%d", argIdx))
			args = append(args, f.Namespace)
			argIdx++
		}
		if f.Kind != "" {
			conditions = append(conditions, fmt.Sprintf("kind = $%d", argIdx))
			args = append(args, f.Kind)
			argIdx++
		}
		if f.Status != "" {
			conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
			args = append(args, f.Status)
			argIdx++
		}
		if f.Scope == "platform" {
			conditions = append(conditions, "organization_id IS NULL")
		} else if f.Scope == "org" {
			conditions = append(conditions, "organization_id IS NOT NULL")
		}
		if f.Search != "" {
			conditions = append(conditions, fmt.Sprintf("(display_text ILIKE $%d OR question_signature ILIKE $%d)", argIdx, argIdx))
			args = append(args, "%"+f.Search+"%")
			argIdx++
		}

		whereClause := ""
		if len(conditions) > 0 {
			whereClause = "WHERE " + strings.Join(conditions, " AND ")
		}

		countQuery := fmt.Sprintf("SELECT count(*) FROM catalog.match_knowledge %s;", whereClause)
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&page.Total); err != nil {
			return err
		}

		query := fmt.Sprintf(`
			SELECT id, organization_id, source_namespace, question_signature, kind,
			       target_product_id, target_granularity, target_fact_fingerprint, candidate_set_fingerprint,
			       policy_version, normalizer_version, ai_config_id, prompt_version, display_text,
			       status, legacy, created_by, created_at, revoked_by, revoked_at, last_used_at
			FROM catalog.match_knowledge
			%s
			ORDER BY id DESC
			LIMIT $%d OFFSET $%d;
		`, whereClause, argIdx, argIdx+1)

		args = append(args, f.Limit, f.Offset)
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			k, scanErr := scanKnowledge(rows)
			if scanErr != nil {
				return scanErr
			}
			page.Items = append(page.Items, k)
		}
		return rows.Err()
	})
	return page, err
}

func scanKnowledge(row pgx.Row) (matching.Knowledge, error) {
	var k matching.Knowledge
	err := row.Scan(
		&k.ID, &k.OrganizationID, &k.SourceNamespace, &k.QuestionSignature, &k.Kind,
		&k.TargetProductID, &k.TargetGranularity, &k.TargetFactFingerprint, &k.CandidateSetFingerprint,
		&k.PolicyVersion, &k.NormalizerVersion, &k.AIConfigID, &k.PromptVersion, &k.DisplayText,
		&k.Status, &k.Legacy, &k.CreatedBy, &k.CreatedAt, &k.RevokedBy, &k.RevokedAt, &k.LastUsedAt,
	)
	return k, err
}

// LoadAliases retrieves in-scope active curated and human-confirmed aliases (US8 T111).
func (r *Repository) LoadAliases(ctx context.Context, orgID int64) (map[string][]int64, error) {
	out := make(map[string][]int64)

	addRows := func(rows pgx.Rows) error {
		for rows.Next() {
			var text string
			var productID int64
			if err := rows.Scan(&text, &productID); err != nil {
				return err
			}
			text = strings.TrimSpace(text)
			if text != "" && productID > 0 {
				out[text] = appendUnique(out[text], productID)
			}
		}
		return rows.Err()
	}

	// 1. Tenant-scoped knowledge (if orgID > 0)
	if orgID > 0 {
		err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
			query := `
				SELECT display_text, target_product_id
				FROM catalog.match_knowledge
				WHERE organization_id = $1
				  AND status = 'active'
				  AND kind IN ('curated_alias', 'human_confirm')
				  AND target_product_id IS NOT NULL
				  AND display_text <> '';
			`
			rows, err := tx.Query(txCtx, query, orgID)
			if err != nil {
				return err
			}
			defer rows.Close()
			return addRows(rows)
		})
		if err != nil {
			return nil, err
		}
	}

	// 2. Platform-scoped knowledge (organization_id IS NULL) under database.AsSystem
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if orgID > 0 {
			var usePlatform bool
			queryPref := `SELECT COALESCE((SELECT use_platform_memory FROM catalog.decision_memory_preferences WHERE organization_id = $1), true)`
			if err := tx.QueryRow(txCtx, queryPref, orgID).Scan(&usePlatform); err != nil {
				return err
			}
			if !usePlatform {
				return nil
			}
		}

		query := `
			SELECT display_text, target_product_id
			FROM catalog.match_knowledge
			WHERE organization_id IS NULL
			  AND status = 'active'
			  AND kind IN ('curated_alias', 'human_confirm')
			  AND target_product_id IS NOT NULL
			  AND display_text <> '';
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		return addRows(rows)
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

func appendUnique(slice []int64, val int64) []int64 {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}
