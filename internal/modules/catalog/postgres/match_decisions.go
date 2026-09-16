package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListMatchDecisions returns paginated decision memories across the platform.
func (r *Repository) ListMatchDecisions(ctx context.Context, search string, limit, offset int) ([]*catalog.MatchDecisionView, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var out []*catalog.MatchDecisionView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"1=1"}
		var args []any

		if search = strings.TrimSpace(search); search != "" {
			args = append(args, "%"+search+"%")
			param := "$" + strconv.Itoa(len(args))
			where = append(where, "(m.norm_name ILIKE "+param+" OR m.reason ILIKE "+param+" OR COALESCE(p.name->>'ar', p.name->>'en', '') ILIKE "+param+" OR COALESCE(p.sku, '') ILIKE "+param+")")
		}

		clause := strings.Join(where, " AND ")

		countQuery := `
			SELECT count(*)
			FROM catalog.match_decisions m
			LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
			WHERE ` + clause + `;
		`
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		query := `
			SELECT m.id, m.organization_id, m.user_id, m.decision_key, m.norm_name, m.chosen_product_id,
			       COALESCE(p.name->>'ar', p.name->>'en', ''), COALESCE(p.sku, ''),
			       m.confidence, COALESCE(m.reason, ''), m.prompt_version, m.hit_count,
			       m.created_at, m.last_used_at
			FROM catalog.match_decisions m
			LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
			WHERE ` + clause + `
			ORDER BY m.last_used_at DESC, m.id DESC
			LIMIT ` + limParam + ` OFFSET ` + offParam + `;
		`
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.MatchDecisionView
			var chosenID *int64
			var orgID *int64
			var userID *int64
			var reason string
			if err := rows.Scan(
				&v.ID, &orgID, &userID, &v.DecisionKey, &v.NormName, &chosenID,
				&v.ChosenProductName, &v.ChosenProductSKU,
				&v.Confidence, &reason, &v.PromptVersion, &v.HitCount,
				&v.CreatedAt, &v.LastUsedAt,
			); err != nil {
				return err
			}
			v.OrganizationID = orgID
			v.UserID = userID
			v.ChosenProductID = chosenID
			v.Reason = reason
			out = append(out, &v)
		}
		return rows.Err()
	})

	return out, total, err
}

// ListMatchDecisionsForOrg returns decision memories strictly scoped to an organization.
func (r *Repository) ListMatchDecisionsForOrg(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.MatchDecisionView, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var out []*catalog.MatchDecisionView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"m.organization_id = $1"}
		args := []any{orgID}

		if search = strings.TrimSpace(search); search != "" {
			args = append(args, "%"+search+"%")
			param := "$" + strconv.Itoa(len(args))
			where = append(where, "(m.norm_name ILIKE "+param+" OR m.reason ILIKE "+param+" OR COALESCE(p.name->>'ar', p.name->>'en', '') ILIKE "+param+" OR COALESCE(p.sku, '') ILIKE "+param+")")
		}

		clause := strings.Join(where, " AND ")

		countQuery := `
			SELECT count(*)
			FROM catalog.match_decisions m
			LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
			WHERE ` + clause + `;
		`
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		query := `
			SELECT m.id, m.organization_id, m.user_id, m.decision_key, m.norm_name, m.chosen_product_id,
			       COALESCE(p.name->>'ar', p.name->>'en', ''), COALESCE(p.sku, ''),
			       m.confidence, COALESCE(m.reason, ''), m.prompt_version, m.hit_count,
			       m.created_at, m.last_used_at
			FROM catalog.match_decisions m
			LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
			WHERE ` + clause + `
			ORDER BY m.last_used_at DESC, m.id DESC
			LIMIT ` + limParam + ` OFFSET ` + offParam + `;
		`
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.MatchDecisionView
			var chosenID *int64
			var dbOrgID *int64
			var dbUserID *int64
			var reason string
			if err := rows.Scan(
				&v.ID, &dbOrgID, &dbUserID, &v.DecisionKey, &v.NormName, &chosenID,
				&v.ChosenProductName, &v.ChosenProductSKU,
				&v.Confidence, &reason, &v.PromptVersion, &v.HitCount,
				&v.CreatedAt, &v.LastUsedAt,
			); err != nil {
				return err
			}
			v.OrganizationID = dbOrgID
			v.UserID = dbUserID
			v.ChosenProductID = chosenID
			v.Reason = reason
			out = append(out, &v)
		}
		return rows.Err()
	})

	return out, total, err
}

// DeleteMatchDecision removes a single match decision from the system cache.
func (r *Repository) DeleteMatchDecision(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `DELETE FROM catalog.match_decisions WHERE id = $1;`, id)
		return err
	})
}

// DeleteMatchDecisionForOrg deletes a match decision only if it belongs to that organization.
func (r *Repository) DeleteMatchDecisionForOrg(ctx context.Context, orgID, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `DELETE FROM catalog.match_decisions WHERE id = $1 AND organization_id = $2;`, id, orgID)
		return err
	})
}

// ClearMatchDecisions purges all cached matching decisions from the system.
func (r *Repository) ClearMatchDecisions(ctx context.Context) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `TRUNCATE TABLE catalog.match_decisions RESTART IDENTITY;`)
		return err
	})
}

// ClearMatchDecisionsForOrg purges all cached matching decisions for a single organization.
func (r *Repository) ClearMatchDecisionsForOrg(ctx context.Context, orgID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.match_decisions WHERE organization_id = $1;`, orgID); err != nil {
			return err
		}
		_, err := tx.Exec(txCtx, `DELETE FROM catalog.customer_product_mappings WHERE organization_id = $1 OR customer_org_id = $1;`, orgID)
		return err
	})
}

// SaveManualDecision records a user-indicated match decision into both match_decisions and customer_product_mappings.
func (r *Repository) SaveManualDecision(ctx context.Context, orgID, userID int64, rawName string, productID int64, reason string) error {
	normName := strings.ToLower(strings.TrimSpace(rawName))
	if normName == "" || productID <= 0 || orgID <= 0 {
		return nil
	}
	if reason == "" {
		reason = i18n.TDefault("w4_mod.s_347_347")
	}
	decKey := "manual:" + normName

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Insert or update in catalog.match_decisions
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.match_decisions (
				organization_id, user_id, decision_key, norm_name, chosen_product_id,
				confidence, reason, prompt_version, hit_count, created_at, last_used_at
			) VALUES (
				$1, NULLIF($2, 0), $3, $4, $5,
				1.000, $6, 'manual:v1', 1, now(), now()
			)
			ON CONFLICT (COALESCE(organization_id, 0), decision_key)
			DO UPDATE SET
				chosen_product_id = EXCLUDED.chosen_product_id,
				confidence = 1.000,
				reason = EXCLUDED.reason,
				user_id = COALESCE(EXCLUDED.user_id, catalog.match_decisions.user_id),
				hit_count = catalog.match_decisions.hit_count + 1,
				last_used_at = now();
		`, orgID, userID, decKey, normName, productID, reason)
		if err != nil {
			return err
		}

		// 2. Also register in catalog.customer_product_mappings for deterministic lookup
		_, err = tx.Exec(txCtx, `
			INSERT INTO catalog.customer_product_mappings (
				organization_id, customer_org_id, raw_name, product_id,
				source, status, is_active, created_at, updated_at
			) VALUES (
				$1, $1, $2, $3,
				'manual', 'processed', true, now(), now()
			)
			ON CONFLICT DO NOTHING;
		`, orgID, rawName, productID)
		return err
	})
}

// IsDecisionMemoryEnabled checks if the decision memory feature is enabled in system settings.
func (r *Repository) IsDecisionMemoryEnabled(ctx context.Context) bool {
	var val any
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT value FROM platform_admin.system_settings WHERE key = 'decision_memory_enabled' LIMIT 1;
		`).Scan(&val)
	})
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

// SetDecisionMemoryEnabled updates the decision memory feature state in system settings.
func (r *Repository) SetDecisionMemoryEnabled(ctx context.Context, enabled bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		val := "true"
		if !enabled {
			val = "false"
		}
		_, err := tx.Exec(txCtx, `
			INSERT INTO platform_admin.system_settings (key, value, description, is_public, updated_at)
			VALUES ('decision_memory_enabled', $1::jsonb, 'Global switch to enable or disable AI Decision Memory across all platform features', true, now())
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();
		`, val)
		return err
	})
}

// ListCustomerMappings returns the saved learned matching decisions for a customer/vendor organization.
func (r *Repository) ListCustomerMappings(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.CustomerMappingView, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var out []*catalog.CustomerMappingView
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"(m.organization_id = $1 OR m.customer_org_id = $1)"}
		args := []any{orgID}

		if search = strings.TrimSpace(search); search != "" {
			args = append(args, "%"+search+"%")
			param := "$" + strconv.Itoa(len(args))
			where = append(where, "(m.raw_name ILIKE "+param+" OR COALESCE(p.name->>'ar', p.name->>'en', '') ILIKE "+param+" OR COALESCE(p.sku, '') ILIKE "+param+")")
		}

		clause := strings.Join(where, " AND ")

		countQuery := `
			SELECT count(*)
			FROM catalog.customer_product_mappings m
			LEFT JOIN catalog.products p ON p.id = m.product_id
			WHERE ` + clause + `;
		`
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		query := `
			SELECT m.id, m.organization_id, m.raw_name, m.product_id,
			       COALESCE(p.name->>'ar', p.name->>'en', ''), COALESCE(p.sku, ''),
			       m.source, m.status, m.created_at, m.updated_at
			FROM catalog.customer_product_mappings m
			LEFT JOIN catalog.products p ON p.id = m.product_id
			WHERE ` + clause + `
			ORDER BY m.updated_at DESC, m.id DESC
			LIMIT ` + limParam + ` OFFSET ` + offParam + `;
		`
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.CustomerMappingView
			if err := rows.Scan(
				&v.ID, &v.OrganizationID, &v.RawName, &v.ProductID,
				&v.ProductName, &v.ProductSKU,
				&v.Source, &v.Status, &v.CreatedAt, &v.UpdatedAt,
			); err != nil {
				return err
			}
			out = append(out, &v)
		}
		return rows.Err()
	})

	return out, total, err
}

// DeleteCustomerMapping removes a saved product mapping for an organization.
func (r *Repository) DeleteCustomerMapping(ctx context.Context, orgID, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			DELETE FROM catalog.customer_product_mappings
			WHERE id = $1 AND (organization_id = $2 OR customer_org_id = $2);
		`, id, orgID)
		return err
	})
}

// ClearCustomerMappings purges all saved product mappings for an organization.
func (r *Repository) ClearCustomerMappings(ctx context.Context, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			DELETE FROM catalog.customer_product_mappings
			WHERE organization_id = $1 OR customer_org_id = $1;
		`, orgID)
		return err
	})
}
