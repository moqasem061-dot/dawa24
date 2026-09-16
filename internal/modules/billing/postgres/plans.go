package postgres

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// CreatePlan inserts a subscription tier and its features.
func (r *Repository) CreatePlan(ctx context.Context, p *billing.Plan) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if p.MaxLoginSessions <= 0 {
			p.MaxLoginSessions = 3
		}
		if p.MaxDevices <= 0 {
			p.MaxDevices = 3
		}
		if p.AIPlanID == "" {
			p.AIPlanID = "plan-basic"
		}
		if p.IsDefault {
			_, _ = tx.Exec(txCtx, `UPDATE billing.plans SET is_default = false;`)
		}
		const query = `
			INSERT INTO billing.plans (slug, name, description, price_month, price_year, duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			RETURNING id, public_id, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query, p.Slug, p.Name, p.Description, p.PriceMonth, p.PriceYear, p.DurationDays, p.MaxUsers, p.MaxLoginSessions, p.MaxDevices, p.AIPlanID, p.IsDefault, p.IsActive).
			Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return err
		}
		for key, val := range p.Features {
			if _, err := tx.Exec(txCtx, `INSERT INTO billing.plan_features (plan_id, feature_key, value) VALUES ($1, $2, $3) ON CONFLICT (plan_id, feature_key) DO UPDATE SET value = EXCLUDED.value;`, p.ID, key, val); err != nil {
				return err
			}
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "reference.plan.create",
			EntityType: "plan",
			EntityID:   strconv.FormatInt(p.ID, 10),
			After:      map[string]any{"slug": p.Slug, "name": p.Name, "price_month": p.PriceMonth},
		})

		return nil
	})
}

// AdminListPlans retrieves all plans (active and inactive) for administrative management.
func (r *Repository) AdminListPlans(ctx context.Context) ([]*billing.Plan, error) {
	var plans []*billing.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, slug, name, description, price_month, price_year,
			       duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active, created_at, updated_at
			FROM billing.plans
			ORDER BY is_default DESC, is_active DESC, id ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p billing.Plan
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Description,
				&p.PriceMonth, &p.PriceYear, &p.DurationDays, &p.MaxUsers,
				&p.MaxLoginSessions, &p.MaxDevices, &p.AIPlanID, &p.IsDefault,
				&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
			plans = append(plans, &p)
		}
		if rows.Err() != nil {
			return rows.Err()
		}

		for _, p := range plans {
			p.Features = loadPlanFeatures(txCtx, tx, p.ID)
		}
		return nil
	})
	return plans, err
}

// UpdatePlan updates an existing subscription tier.
func (r *Repository) UpdatePlan(ctx context.Context, p *billing.Plan) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if p.MaxLoginSessions <= 0 {
			p.MaxLoginSessions = 3
		}
		if p.MaxDevices <= 0 {
			p.MaxDevices = 3
		}
		if p.AIPlanID == "" {
			p.AIPlanID = "plan-basic"
		}
		if p.IsDefault {
			_, _ = tx.Exec(txCtx, `UPDATE billing.plans SET is_default = false WHERE id <> $1;`, p.ID)
		}
		const query = `
			UPDATE billing.plans
			SET slug = COALESCE(NULLIF($1, ''), slug),
			    name = $2, description = $3, price_month = $4, price_year = $5, duration_days = $6,
			    max_users = $7, max_login_sessions = $8, max_devices = $9, ai_plan_id = $10, is_default = $11,
			    is_active = $12, updated_at = now()
			WHERE id = $13;
		`
		tag, err := tx.Exec(txCtx, query, p.Slug, p.Name, p.Description, p.PriceMonth, p.PriceYear, p.DurationDays, p.MaxUsers, p.MaxLoginSessions, p.MaxDevices, p.AIPlanID, p.IsDefault, p.IsActive, p.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("plan")
		}
		_, _ = tx.Exec(txCtx, `DELETE FROM billing.plan_features WHERE plan_id = $1;`, p.ID)
		for key, val := range p.Features {
			if _, err := tx.Exec(txCtx, `INSERT INTO billing.plan_features (plan_id, feature_key, value) VALUES ($1, $2, $3) ON CONFLICT (plan_id, feature_key) DO UPDATE SET value = EXCLUDED.value;`, p.ID, key, val); err != nil {
				return err
			}
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "reference.plan.edit",
			EntityType: "plan",
			EntityID:   strconv.FormatInt(p.ID, 10),
			After:      map[string]any{"slug": p.Slug, "name": p.Name, "price_month": p.PriceMonth},
		})

		return nil
	})
}

// TogglePlanActive toggles the is_active status of a plan.
func (r *Repository) TogglePlanActive(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE billing.plans SET is_active = NOT is_active, updated_at = now() WHERE id = $1;`
		tag, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("plan")
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "reference.plan.toggle",
			EntityType: "plan",
			EntityID:   strconv.FormatInt(id, 10),
		})

		return nil
	})
}

// SetDefaultPlan sets a plan as the system-wide default and ensures it is active.
func (r *Repository) SetDefaultPlan(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE billing.plans SET is_default = (id = $1), is_active = CASE WHEN id = $1 THEN true ELSE is_active END, updated_at = now();`, id)
		return err
	})
}

// DeletePlan deletes a plan if it's not the default and has no active subscriptions.
func (r *Repository) DeletePlan(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var isDefault bool
		err := tx.QueryRow(txCtx, `SELECT is_default FROM billing.plans WHERE id = $1;`, id).Scan(&isDefault)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("plan")
			}
			return err
		}
		if isDefault {
			return apperr.Validation("plan.delete_default", i18n.TDefault("w4_mod.w4str_73_73"), nil)
		}

		var activeSubs int
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM billing.subscriptions WHERE plan_id = $1 AND status = 'active' AND expires_at > now();`, id).Scan(&activeSubs)
		if activeSubs > 0 {
			return apperr.Validation("plan.has_active_subscriptions", i18n.TDefault("w4_mod.w4str_74_74"), nil)
		}

		_, _ = tx.Exec(txCtx, `DELETE FROM billing.plan_features WHERE plan_id = $1;`, id)
		_, err = tx.Exec(txCtx, `DELETE FROM billing.plans WHERE id = $1;`, id)
		return err
	})
}

// GetDefaultPlan retrieves the default subscription plan.
func (r *Repository) GetDefaultPlan(ctx context.Context) (*billing.Plan, error) {
	var p billing.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, slug, name, description, price_month, price_year,
			       duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active, created_at, updated_at
			FROM billing.plans
			WHERE is_default = true AND is_active = true
			ORDER BY id ASC
			LIMIT 1;
		`
		err := tx.QueryRow(txCtx, query).Scan(
			&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Description,
			&p.PriceMonth, &p.PriceYear, &p.DurationDays, &p.MaxUsers,
			&p.MaxLoginSessions, &p.MaxDevices, &p.AIPlanID, &p.IsDefault,
			&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil && database.IsNotFound(err) {
			// Fallback to first active plan
			fallbackQuery := `
				SELECT id, public_id, slug, name, description, price_month, price_year,
				       duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active, created_at, updated_at
				FROM billing.plans
				WHERE is_active = true
				ORDER BY id ASC
				LIMIT 1;
			`
			if err := tx.QueryRow(txCtx, fallbackQuery).Scan(
				&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Description,
				&p.PriceMonth, &p.PriceYear, &p.DurationDays, &p.MaxUsers,
				&p.MaxLoginSessions, &p.MaxDevices, &p.AIPlanID, &p.IsDefault,
				&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		p.Features = loadPlanFeatures(txCtx, tx, p.ID)
		return nil
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("plan")
		}
		return nil, err
	}
	return &p, nil
}

// loadPlanFeatures reads the feature key-value pairs for a plan from billing.plan_features.
func loadPlanFeatures(ctx context.Context, tx pgx.Tx, planID int64) map[string]string {
	features := make(map[string]string)
	rows, err := tx.Query(ctx, `SELECT feature_key, value FROM billing.plan_features WHERE plan_id = $1;`, planID)
	if err != nil {
		return features
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			features[k] = v
		}
	}
	return features
}
