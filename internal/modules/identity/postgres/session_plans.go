package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
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

// PurchaseSessionPlan runs one purchase in a single transaction. The plan row is
// read under a shared lock, so its price cannot change under the charge. decide
// then charges the payer's wallet and reports whether this was a replay. The
// buyer's limit is written last, and only when decide did not report a replay.
// A refusal anywhere rolls the whole purchase back.
func (r *Repository) PurchaseSessionPlan(ctx context.Context, buyerUserID, planID int64, decide identity.SessionPlanDecider) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var plan identity.SessionPlan
		err := tx.QueryRow(txCtx, `SELECT id, name, max_login_sessions, price, duration_days, is_free, is_active, created_at, updated_at
			FROM identity.session_plans WHERE id = $1 FOR SHARE;`, planID).Scan(
			&plan.ID, &plan.Name, &plan.MaxLoginSessions, &plan.Price, &plan.DurationDays,
			&plan.IsFree, &plan.IsActive, &plan.CreatedAt, &plan.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("session_plan")
			}
			return err
		}
		replayed, err := decide(txCtx, tx, &plan)
		if err != nil {
			return err
		}
		if replayed {
			return nil
		}
		// Upsert, so a buyer with no security row still gets the limit that was paid for.
		_, err = tx.Exec(txCtx, `INSERT INTO identity.user_security (user_id, max_login_sessions) VALUES ($1, $2)
			ON CONFLICT (user_id) DO UPDATE SET max_login_sessions = EXCLUDED.max_login_sessions;`, buyerUserID, plan.MaxLoginSessions)
		return err
	})
}
