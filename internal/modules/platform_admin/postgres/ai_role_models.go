package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListAIRoleModels returns all role-to-model configurations ordered by role.
func (r *Repository) ListAIRoleModels(ctx context.Context) ([]*platformadmin.AIRoleModel, error) {
	var items []*platformadmin.AIRoleModel
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT role, model, is_active, max_tokens, notes, updated_by, updated_at
			FROM platform_admin.ai_role_models
			ORDER BY role ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m platformadmin.AIRoleModel
			if err := rows.Scan(&m.Role, &m.Model, &m.IsActive, &m.MaxTokens, &m.Notes, &m.UpdatedBy, &m.UpdatedAt); err != nil {
				return err
			}
			items = append(items, &m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// GetAIRoleModel returns the configuration for a specific role.
func (r *Repository) GetAIRoleModel(ctx context.Context, role string) (*platformadmin.AIRoleModel, error) {
	var m platformadmin.AIRoleModel
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT role, model, is_active, max_tokens, notes, updated_by, updated_at
			FROM platform_admin.ai_role_models
			WHERE role = $1;
		`
		err := tx.QueryRow(txCtx, query, role).Scan(
			&m.Role, &m.Model, &m.IsActive, &m.MaxTokens, &m.Notes, &m.UpdatedBy, &m.UpdatedAt,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("ai_role_model")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// SaveAIRoleModel inserts or updates an AI role-to-model configuration.
func (r *Repository) SaveAIRoleModel(ctx context.Context, rm *platformadmin.AIRoleModel) error {
	if rm == nil || rm.Role == "" || rm.Model == "" {
		return apperr.Validation("ai.invalid_role_model", "role and model are required", nil)
	}
	now := time.Now()
	rm.UpdatedAt = now

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform_admin.ai_role_models (role, model, is_active, max_tokens, notes, updated_by, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (role) DO UPDATE SET
				model = EXCLUDED.model,
				is_active = EXCLUDED.is_active,
				max_tokens = EXCLUDED.max_tokens,
				notes = EXCLUDED.notes,
				updated_by = EXCLUDED.updated_by,
				updated_at = EXCLUDED.updated_at;
		`
		_, err := tx.Exec(txCtx, query, rm.Role, rm.Model, rm.IsActive, rm.MaxTokens, rm.Notes, rm.UpdatedBy, rm.UpdatedAt)
		return err
	})
}

// DeleteAIRoleModel removes an AI role model mapping.
func (r *Repository) DeleteAIRoleModel(ctx context.Context, role string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM platform_admin.ai_role_models WHERE role = $1;`
		_, err := tx.Exec(txCtx, query, role)
		return err
	})
}
