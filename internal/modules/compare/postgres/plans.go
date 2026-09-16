package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

const planColumns = `id, public_id, name, slug, description, price_monthly, price_yearly, price_lifetime, currency, trial_days, is_active, is_public, is_recommended, sort_order, created_by, updated_by, created_at, updated_at, deleted_at`

func scanPlan(row pgx.Row) (*compare.Plan, error) {
	var p compare.Plan
	if err := row.Scan(
		&p.ID, &p.PublicID, &p.Name, &p.Slug, &p.Description,
		&p.PriceMonthly, &p.PriceYearly, &p.PriceLifetime, &p.Currency,
		&p.TrialDays, &p.IsActive, &p.IsPublic, &p.IsRecommended,
		&p.SortOrder, &p.CreatedBy, &p.UpdatedBy,
		&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) ListPlans(ctx context.Context, onlyPublic bool) ([]*compare.Plan, error) {
	var list []*compare.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + planColumns + `
			FROM compare.plans
			WHERE deleted_at IS NULL AND ($1 = false OR (is_active = true AND is_public = true))
			ORDER BY sort_order ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query, onlyPublic)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			p, err := scanPlan(rows)
			if err != nil {
				return err
			}
			list = append(list, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("compare postgres: list plans: %w", err)
	}

	for _, p := range list {
		features, _ := r.ListPlanFeatures(ctx, p.ID)
		p.Features = features
	}
	return list, nil
}

func (r *Repository) GetPlanByID(ctx context.Context, id int64) (*compare.Plan, error) {
	var p *compare.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + planColumns + ` FROM compare.plans WHERE id = $1 AND deleted_at IS NULL;`
		var err error
		p, err = scanPlan(tx.QueryRow(txCtx, query, id))
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("plan")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	features, _ := r.ListPlanFeatures(ctx, p.ID)
	p.Features = features
	return p, nil
}

func (r *Repository) GetPlanBySlug(ctx context.Context, slug string) (*compare.Plan, error) {
	var p *compare.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + planColumns + ` FROM compare.plans WHERE slug = $1 AND deleted_at IS NULL;`
		var err error
		p, err = scanPlan(tx.QueryRow(txCtx, query, slug))
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("plan")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	features, _ := r.ListPlanFeatures(ctx, p.ID)
	p.Features = features
	return p, nil
}

func (r *Repository) CreatePlan(ctx context.Context, plan *compare.Plan) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO compare.plans (
				name, slug, description, price_monthly, price_yearly, price_lifetime,
				currency, trial_days, is_active, is_public, is_recommended, sort_order, created_by, updated_by
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			plan.Name, plan.Slug, plan.Description, plan.PriceMonthly, plan.PriceYearly, plan.PriceLifetime,
			plan.Currency, plan.TrialDays, plan.IsActive, plan.IsPublic, plan.IsRecommended, plan.SortOrder,
			plan.CreatedBy, plan.UpdatedBy,
		).Scan(&plan.ID, &plan.PublicID, &plan.CreatedAt, &plan.UpdatedAt)
	})
}

func (r *Repository) UpdatePlan(ctx context.Context, plan *compare.Plan) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE compare.plans
			SET name = $2, slug = $3, description = $4, price_monthly = $5, price_yearly = $6, price_lifetime = $7,
			    currency = $8, trial_days = $9, is_active = $10, is_public = $11, is_recommended = $12,
			    sort_order = $13, updated_by = $14, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		res, err := tx.Exec(txCtx, query,
			plan.ID, plan.Name, plan.Slug, plan.Description, plan.PriceMonthly, plan.PriceYearly, plan.PriceLifetime,
			plan.Currency, plan.TrialDays, plan.IsActive, plan.IsPublic, plan.IsRecommended, plan.SortOrder, plan.UpdatedBy,
		)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("plan")
		}
		return nil
	})
}

func (r *Repository) DeletePlan(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.plans SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;`, id)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("plan")
		}
		return nil
	})
}

func (r *Repository) ListPlanFeatures(ctx context.Context, planID int64) ([]*compare.PlanFeature, error) {
	var list []*compare.PlanFeature
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, plan_id, key, name, description, value, value_type, is_active, sort_order, created_at, updated_at
			FROM compare.plan_features
			WHERE plan_id = $1 AND deleted_at IS NULL
			ORDER BY sort_order ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query, planID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var f compare.PlanFeature
			if err := rows.Scan(
				&f.ID, &f.PlanID, &f.Key, &f.Name, &f.Description,
				&f.Value, &f.ValueType, &f.IsActive, &f.SortOrder,
				&f.CreatedAt, &f.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &f)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) SetPlanFeature(ctx context.Context, feature *compare.PlanFeature) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO compare.plan_features (plan_id, key, name, description, value, value_type, is_active, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (plan_id, key) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				value = EXCLUDED.value,
				value_type = EXCLUDED.value_type,
				is_active = EXCLUDED.is_active,
				sort_order = EXCLUDED.sort_order,
				updated_at = now()
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			feature.PlanID, feature.Key, feature.Name, feature.Description,
			feature.Value, feature.ValueType, feature.IsActive, feature.SortOrder,
		).Scan(&feature.ID, &feature.CreatedAt, &feature.UpdatedAt)
	})
}

func (r *Repository) DeletePlanFeature(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.plan_features SET deleted_at = now() WHERE id = $1;`, id)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("plan feature")
		}
		return nil
	})
}

func (r *Repository) CreatePlanRequest(ctx context.Context, req *compare.PlanRequest) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO compare.plan_requests (plan_id, organization_id, user_id, status, notes)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			req.PlanID, req.OrganizationID, req.UserID, string(req.Status), req.Notes,
		).Scan(&req.ID, &req.PublicID, &req.CreatedAt, &req.UpdatedAt)
	})
}

func (r *Repository) GetPlanRequestByID(ctx context.Context, id int64) (*compare.PlanRequest, error) {
	var req compare.PlanRequest
	var statusStr string
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, plan_id, organization_id, user_id, status, notes, rejection_reason, reviewed_by, reviewed_at, created_at, updated_at, deleted_at
			FROM compare.plan_requests
			WHERE id = $1 AND deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&req.ID, &req.PublicID, &req.PlanID, &req.OrganizationID, &req.UserID,
			&statusStr, &req.Notes, &req.RejectionReason, &req.ReviewedBy, &req.ReviewedAt,
			&req.CreatedAt, &req.UpdatedAt, &req.DeletedAt,
		)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperr.NotFound("plan request")
		}
		return nil, err
	}
	req.Status = compare.PlanRequestStatus(statusStr)
	return &req, nil
}

func (r *Repository) ListPlanRequestsByOrg(ctx context.Context, orgID int64) ([]*compare.PlanRequest, error) {
	var list []*compare.PlanRequest
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, plan_id, organization_id, user_id, status, notes, rejection_reason, reviewed_by, reviewed_at, created_at, updated_at, deleted_at
			FROM compare.plan_requests
			WHERE organization_id = $1 AND deleted_at IS NULL
			ORDER BY created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var req compare.PlanRequest
			var statusStr string
			if err := rows.Scan(
				&req.ID, &req.PublicID, &req.PlanID, &req.OrganizationID, &req.UserID,
				&statusStr, &req.Notes, &req.RejectionReason, &req.ReviewedBy, &req.ReviewedAt,
				&req.CreatedAt, &req.UpdatedAt, &req.DeletedAt,
			); err != nil {
				return err
			}
			req.Status = compare.PlanRequestStatus(statusStr)
			list = append(list, &req)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) ListPendingPlanRequests(ctx context.Context) ([]*compare.PlanRequest, error) {
	var list []*compare.PlanRequest
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, plan_id, organization_id, user_id, status, notes, rejection_reason, reviewed_by, reviewed_at, created_at, updated_at, deleted_at
			FROM compare.plan_requests
			WHERE status = 'pending' AND deleted_at IS NULL
			ORDER BY created_at ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var req compare.PlanRequest
			var statusStr string
			if err := rows.Scan(
				&req.ID, &req.PublicID, &req.PlanID, &req.OrganizationID, &req.UserID,
				&statusStr, &req.Notes, &req.RejectionReason, &req.ReviewedBy, &req.ReviewedAt,
				&req.CreatedAt, &req.UpdatedAt, &req.DeletedAt,
			); err != nil {
				return err
			}
			req.Status = compare.PlanRequestStatus(statusStr)
			list = append(list, &req)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) ReviewPlanRequest(ctx context.Context, id int64, status compare.PlanRequestStatus, reviewerID int64, reason string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		now := time.Now().UTC()
		query := `
			UPDATE compare.plan_requests
			SET status = $2, reviewed_by = $3, reviewed_at = $4, rejection_reason = $5, updated_at = $4
			WHERE id = $1 AND deleted_at IS NULL;
		`
		res, err := tx.Exec(txCtx, query, id, string(status), reviewerID, now, reason)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("plan request")
		}
		return nil
	})
}
