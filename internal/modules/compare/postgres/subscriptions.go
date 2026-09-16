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

const subColumns = `s.id, s.public_id, s.plan_id, s.organization_id, s.user_id, s.billing_period, s.payment_method, s.starts_at, s.ends_at, s.status, s.seats, s.created_at, s.updated_at, s.deleted_at`

func scanSubscription(row pgx.Row) (*compare.Subscription, error) {
	var s compare.Subscription
	var statusStr string
	if err := row.Scan(
		&s.ID, &s.PublicID, &s.PlanID, &s.OrganizationID, &s.UserID,
		&s.BillingPeriod, &s.PaymentMethod, &s.StartsAt, &s.EndsAt,
		&statusStr, &s.Seats, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
	); err != nil {
		return nil, err
	}
	s.Status = compare.SubscriptionStatus(statusStr)
	return &s, nil
}

func (r *Repository) CreateSubscription(ctx context.Context, sub *compare.Subscription) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO compare.subscriptions (
				plan_id, organization_id, user_id, billing_period, payment_method, starts_at, ends_at, status, seats
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, public_id, created_at, updated_at;
		`
		if sub.Seats <= 0 {
			sub.Seats = 1
		}
		if sub.StartsAt.IsZero() {
			sub.StartsAt = time.Now().UTC()
		}
		if sub.Status == "" {
			sub.Status = compare.SubActive
		}
		err := tx.QueryRow(txCtx, query,
			sub.PlanID, sub.OrganizationID, sub.UserID, sub.BillingPeriod, sub.PaymentMethod,
			sub.StartsAt, sub.EndsAt, string(sub.Status), sub.Seats,
		).Scan(&sub.ID, &sub.PublicID, &sub.CreatedAt, &sub.UpdatedAt)
		if err != nil {
			return err
		}

		// Auto-assign owner to seat
		assignQuery := `
			INSERT INTO compare.subscription_users (subscription_id, user_id, is_active)
			VALUES ($1, $2, true)
			ON CONFLICT (subscription_id, user_id) DO UPDATE SET is_active = true, updated_at = now();
		`
		_, err = tx.Exec(txCtx, assignQuery, sub.ID, sub.UserID)
		return err
	})
}

func (r *Repository) GetSubscriptionByID(ctx context.Context, id int64) (*compare.Subscription, error) {
	var s *compare.Subscription
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + subColumns + ` FROM compare.subscriptions s WHERE s.id = $1 AND s.deleted_at IS NULL;`
		var err error
		s, err = scanSubscription(tx.QueryRow(txCtx, query, id))
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("subscription")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	plan, err := r.GetPlanByID(ctx, s.PlanID)
	if err == nil {
		s.Plan = plan
	}
	return s, nil
}

func (r *Repository) GetActiveSubscription(ctx context.Context, userID int64, orgID *int64) (*compare.Subscription, error) {
	var s *compare.Subscription
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + subColumns + `
			FROM compare.subscriptions s
			LEFT JOIN compare.subscription_users su ON s.id = su.subscription_id AND su.deleted_at IS NULL
			WHERE s.status = 'active'
			  AND (s.ends_at IS NULL OR s.ends_at > now())
			  AND s.deleted_at IS NULL
			  AND (
			      (su.user_id = $1 AND su.is_active = true)
			      OR ($2::bigint IS NOT NULL AND s.organization_id = $2 AND s.user_id = $1)
			  )
			ORDER BY s.starts_at DESC
			LIMIT 1;
		`
		var err error
		s, err = scanSubscription(tx.QueryRow(txCtx, query, userID, orgID))
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("active subscription")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	plan, err := r.GetPlanByID(ctx, s.PlanID)
	if err == nil {
		s.Plan = plan
	}
	return s, nil
}

func (r *Repository) ListSubscriptionsByOrg(ctx context.Context, orgID int64) ([]*compare.Subscription, error) {
	var list []*compare.Subscription
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + subColumns + `
			FROM compare.subscriptions s
			WHERE s.organization_id = $1 AND s.deleted_at IS NULL
			ORDER BY s.starts_at DESC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			s, err := scanSubscription(rows)
			if err != nil {
				return err
			}
			list = append(list, s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("compare postgres: list subscriptions: %w", err)
	}
	for _, s := range list {
		plan, _ := r.GetPlanByID(ctx, s.PlanID)
		s.Plan = plan
	}
	return list, nil
}

func (r *Repository) UpdateSubscriptionStatus(ctx context.Context, id int64, status compare.SubscriptionStatus) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.subscriptions SET status = $2, updated_at = now() WHERE id = $1 AND deleted_at IS NULL;`, id, string(status))
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("subscription")
		}
		return nil
	})
}

func (r *Repository) AssignSubscriptionUser(ctx context.Context, subID int64, userID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var seats int
		err := tx.QueryRow(txCtx, `SELECT seats FROM compare.subscriptions WHERE id = $1 AND deleted_at IS NULL;`, subID).Scan(&seats)
		if err != nil {
			return err
		}

		var count int
		err = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM compare.subscription_users WHERE subscription_id = $1 AND is_active = true AND deleted_at IS NULL;`, subID).Scan(&count)
		if err != nil {
			return err
		}

		if count >= seats {
			return apperr.Conflict("subscription.seats_full", "All available subscription seats are currently occupied.")
		}

		query := `
			INSERT INTO compare.subscription_users (subscription_id, user_id, is_active)
			VALUES ($1, $2, true)
			ON CONFLICT (subscription_id, user_id) DO UPDATE SET is_active = true, deleted_at = NULL, updated_at = now();
		`
		_, err = tx.Exec(txCtx, query, subID, userID)
		return err
	})
}

func (r *Repository) RemoveSubscriptionUser(ctx context.Context, subID int64, userID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.subscription_users SET is_active = false, deleted_at = now() WHERE subscription_id = $1 AND user_id = $2;`, subID, userID)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("subscription user")
		}
		return nil
	})
}

func (r *Repository) ListSubscriptionUsers(ctx context.Context, subID int64) ([]*compare.SubscriptionUser, error) {
	var list []*compare.SubscriptionUser
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, subscription_id, user_id, is_active, created_at, updated_at
			FROM compare.subscription_users
			WHERE subscription_id = $1 AND deleted_at IS NULL
			ORDER BY created_at ASC;
		`
		rows, err := tx.Query(txCtx, query, subID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var su compare.SubscriptionUser
			if err := rows.Scan(&su.ID, &su.SubscriptionID, &su.UserID, &su.IsActive, &su.CreatedAt, &su.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &su)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) IsUserAssignedToSubscription(ctx context.Context, subID int64, userID int64) (bool, error) {
	var exists bool
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT EXISTS (
				SELECT 1 FROM compare.subscription_users
				WHERE subscription_id = $1 AND user_id = $2 AND is_active = true AND deleted_at IS NULL
			);
		`
		return tx.QueryRow(txCtx, query, subID, userID).Scan(&exists)
	})
	return exists, err
}
