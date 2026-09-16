package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// GetDecisionMemoryPreference returns whether the organization uses platform decision memory. Defaults to true.
func (r *Repository) GetDecisionMemoryPreference(ctx context.Context, orgID int64) (bool, error) {
	if orgID <= 0 {
		return true, nil
	}
	var usePlatform bool
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT use_platform_memory
			FROM catalog.decision_memory_preferences
			WHERE organization_id = $1;
		`, orgID).Scan(&usePlatform)
	})
	if err == pgx.ErrNoRows {
		return true, nil
	}
	return usePlatform, err
}

// SetDecisionMemoryPreference updates the organization's preference for using platform memory.
func (r *Repository) SetDecisionMemoryPreference(ctx context.Context, orgID int64, usePlatform bool, updatedBy int64) error {
	if orgID <= 0 {
		return nil
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.decision_memory_preferences (
				organization_id, use_platform_memory, updated_by, updated_at
			) VALUES ($1, $2, NULLIF($3, 0), now())
			ON CONFLICT (organization_id) DO UPDATE SET
				use_platform_memory = EXCLUDED.use_platform_memory,
				updated_by = EXCLUDED.updated_by,
				updated_at = now();
		`, orgID, usePlatform, updatedBy)
		return err
	})
}

// PromoteMatchDecision elevates an organization's decision to platform-wide scope.
func (r *Repository) PromoteMatchDecision(ctx context.Context, id int64, adminUserID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE catalog.match_decisions
			SET scope = 'platform', promoted_by = NULLIF($2, 0), promoted_at = now()
			WHERE id = $1;
		`, id, adminUserID)
		return err
	})
}

// DemoteMatchDecision reverts a decision back to organization-only scope.
func (r *Repository) DemoteMatchDecision(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE catalog.match_decisions
			SET scope = 'org', promoted_by = NULL, promoted_at = NULL
			WHERE id = $1;
		`, id)
		return err
	})
}

// BulkPromoteMatchDecisions promotes multiple decisions to platform scope.
func (r *Repository) BulkPromoteMatchDecisions(ctx context.Context, ids []int64, adminUserID int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var affected int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `
			UPDATE catalog.match_decisions
			SET scope = 'platform', promoted_by = NULLIF($2, 0), promoted_at = now()
			WHERE id = ANY($1::bigint[]);
		`, ids, adminUserID)
		if err != nil {
			return err
		}
		affected = res.RowsAffected()
		return nil
	})
	return affected, err
}

// BulkDeleteMatchDecisions removes multiple decisions from the cache.
func (r *Repository) BulkDeleteMatchDecisions(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var affected int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `
			DELETE FROM catalog.match_decisions
			WHERE id = ANY($1::bigint[]);
		`, ids)
		if err != nil {
			return err
		}
		affected = res.RowsAffected()
		return nil
	})
	return affected, err
}

// RelinkMatchDecision updates the chosen product on a decision record as administrator.
func (r *Repository) RelinkMatchDecision(ctx context.Context, id int64, productID *int64, adminUserID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var normName string
		err := tx.QueryRow(txCtx, `
			UPDATE catalog.match_decisions
			SET chosen_product_id = $2,
			    confidence = 1.000,
			    source = 'admin',
			    user_id = COALESCE(NULLIF($3, 0), user_id),
			    last_used_at = now()
			WHERE id = $1
			RETURNING norm_name;
		`, id, productID, adminUserID).Scan(&normName)
		if err != nil {
			return err
		}
		if productID != nil && *productID > 0 && normName != "" {
			if _, err := tx.Exec(txCtx, `
				INSERT INTO catalog.product_aliases (product_id, alias, source, confidence)
				VALUES ($1, $2, 'manual', 1.000)
				ON CONFLICT (alias, product_id) DO NOTHING;
			`, *productID, normName); err != nil {
				return err
			}
		}
		return nil
	})
}

// RelinkMatchDecisionForOrg updates a decision owned by an organization.
func (r *Repository) RelinkMatchDecisionForOrg(ctx context.Context, orgID int64, id int64, productID *int64, userID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var normName string
		err := tx.QueryRow(txCtx, `
			UPDATE catalog.match_decisions
			SET chosen_product_id = $3,
			    confidence = 1.000,
			    source = 'manual',
			    user_id = COALESCE(NULLIF($4, 0), user_id),
			    last_used_at = now()
			WHERE id = $1 AND organization_id = $2
			RETURNING norm_name;
		`, id, orgID, productID, userID).Scan(&normName)
		if err != nil {
			return err
		}
		if productID != nil && *productID > 0 && normName != "" {
			_, _ = tx.Exec(txCtx, `
				INSERT INTO catalog.customer_product_mappings (
					organization_id, customer_org_id, raw_name, product_id,
					source, status, is_active, created_at, updated_at
				) VALUES (
					$1, $1, $2, $3,
					'manual', 'processed', true, now(), now()
				)
				ON CONFLICT DO NOTHING;
			`, orgID, normName, *productID)
		}
		return nil
	})
}
