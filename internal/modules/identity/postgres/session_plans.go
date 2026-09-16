package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListSessionPlans returns active concurrent sign-in plans.
func (r *Repository) ListSessionPlans(ctx context.Context) ([]*identity.SessionPlan, error) {
	var list []*identity.SessionPlan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT id, name, max_login_sessions, price, duration_days, is_free, is_active, created_at, updated_at FROM identity.session_plans WHERE is_active = true ORDER BY max_login_sessions ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p identity.SessionPlan
			if err := rows.Scan(&p.ID, &p.Name, &p.MaxLoginSessions, &p.Price, &p.DurationDays, &p.IsFree, &p.IsActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &p)
		}
		return rows.Err()
	})
	return list, err
}

// GetSessionPlanByID fetches one plan.
func (r *Repository) GetSessionPlanByID(ctx context.Context, id int64) (*identity.SessionPlan, error) {
	var p identity.SessionPlan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT id, name, max_login_sessions, price, duration_days, is_free, is_active, created_at, updated_at FROM identity.session_plans WHERE id = $1;`
		err := tx.QueryRow(txCtx, query, id).Scan(&p.ID, &p.Name, &p.MaxLoginSessions, &p.Price, &p.DurationDays, &p.IsFree, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("session_plan")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SetMaxLoginSessions updates the user's concurrent sign-in limit.
func (r *Repository) SetMaxLoginSessions(ctx context.Context, userID int64, max int) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE identity.user_security SET max_login_sessions = $1 WHERE user_id = $2;`, max, userID)
		return err
	})
}
