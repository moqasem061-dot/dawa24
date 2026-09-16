package postgres

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// GetPlanBySlug retrieves a plan by slug.
func (r *Repository) GetPlanBySlug(ctx context.Context, slug string) (*billing.Plan, error) {
	var p billing.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, slug, name, description, price_month, price_year,
			       duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active, created_at, updated_at
			FROM billing.plans
			WHERE slug = $1 AND is_active = true;
		`
		if err := tx.QueryRow(txCtx, query, slug).Scan(
			&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Description,
			&p.PriceMonth, &p.PriceYear, &p.DurationDays, &p.MaxUsers,
			&p.MaxLoginSessions, &p.MaxDevices, &p.AIPlanID, &p.IsDefault,
			&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
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

// CreateSubscription activates a subscription for a user/org.
func (r *Repository) CreateSubscription(ctx context.Context, sub *billing.Subscription) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if sub.BillingCycle == "" {
			sub.BillingCycle = "monthly"
		}

		// A user/organisation has exactly one live platform subscription. Booking a
		// new one (upgrade, downgrade, plan switch, or renewal onto a different
		// plan) supersedes any earlier live row so that the "current plan" reads —
		// which pick the most recent live subscription — resolve to the plan just
		// paid for, instead of an older row whose expires_at happens to sit
		// further in the future (e.g. an unexpired annual term replaced by a
		// monthly one).
		if sub.OrganizationID != nil && *sub.OrganizationID > 0 {
			if _, err := tx.Exec(txCtx, `
				UPDATE billing.subscriptions
				SET status = 'expired', auto_renew = false, updated_at = now()
				WHERE organization_id = $1
				  AND status IN ('active', 'trialing', 'past_due');
			`, *sub.OrganizationID); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(txCtx, `
				UPDATE billing.subscriptions
				SET status = 'expired', auto_renew = false, updated_at = now()
				WHERE user_id = $1
				  AND organization_id IS NULL
				  AND status IN ('active', 'trialing', 'past_due');
			`, sub.UserID); err != nil {
				return err
			}
		}

		query := `
			INSERT INTO billing.subscriptions (
				user_id, organization_id, plan_id, status, billing_cycle, auto_renew,
				starts_at, expires_at, last_renewed_at, renewal_attempts, source_system, source_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			sub.UserID, sub.OrganizationID, sub.PlanID, string(sub.Status),
			sub.BillingCycle, sub.AutoRenew, sub.StartsAt, sub.ExpiresAt,
			sub.LastRenewedAt, sub.RenewalAttempts, sub.SourceSystem, sub.SourceID,
		).Scan(&sub.ID, &sub.PublicID, &sub.CreatedAt, &sub.UpdatedAt)
	})
}

// GetActiveSubscription retrieves current active subscription for a user.
func (r *Repository) GetActiveSubscription(ctx context.Context, userID int64) (*billing.Subscription, error) {
	var sub billing.Subscription
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, plan_id, status,
			       COALESCE(billing_cycle, 'monthly'), COALESCE(auto_renew, false),
			       starts_at, expires_at, last_renewed_at, COALESCE(renewal_attempts, 0),
			       source_system, source_id, created_at, updated_at
			FROM billing.subscriptions
			WHERE user_id = $1 AND status = 'active' AND expires_at > now()
			ORDER BY starts_at DESC, id DESC
			LIMIT 1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, userID).Scan(
			&sub.ID, &sub.PublicID, &sub.UserID, &sub.OrganizationID, &sub.PlanID,
			&statusStr, &sub.BillingCycle, &sub.AutoRenew,
			&sub.StartsAt, &sub.ExpiresAt, &sub.LastRenewedAt, &sub.RenewalAttempts,
			&sub.SourceSystem, &sub.SourceID, &sub.CreatedAt, &sub.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("subscription")
			}
			return err
		}
		sub.Status = billing.SubscriptionStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// GetActiveSubscriptionByOrg retrieves current active subscription for an organization.
func (r *Repository) GetActiveSubscriptionByOrg(ctx context.Context, orgID int64) (*billing.Subscription, error) {
	var sub billing.Subscription
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, plan_id, status,
			       COALESCE(billing_cycle, 'monthly'), COALESCE(auto_renew, false),
			       starts_at, expires_at, last_renewed_at, COALESCE(renewal_attempts, 0),
			       source_system, source_id, created_at, updated_at
			FROM billing.subscriptions
			WHERE organization_id = $1 AND status = 'active' AND expires_at > now()
			ORDER BY starts_at DESC, id DESC
			LIMIT 1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, orgID).Scan(
			&sub.ID, &sub.PublicID, &sub.UserID, &sub.OrganizationID, &sub.PlanID,
			&statusStr, &sub.BillingCycle, &sub.AutoRenew,
			&sub.StartsAt, &sub.ExpiresAt, &sub.LastRenewedAt, &sub.RenewalAttempts,
			&sub.SourceSystem, &sub.SourceID, &sub.CreatedAt, &sub.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("subscription")
			}
			return err
		}
		sub.Status = billing.SubscriptionStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// ListDueSubscriptionsForRenewal returns subscriptions where auto_renew is enabled and expires_at is due.
func (r *Repository) ListDueSubscriptionsForRenewal(ctx context.Context, now time.Time) ([]*billing.Subscription, error) {
	var list []*billing.Subscription
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, plan_id, status,
			       COALESCE(billing_cycle, 'monthly'), COALESCE(auto_renew, false),
			       starts_at, expires_at, last_renewed_at, COALESCE(renewal_attempts, 0),
			       source_system, source_id, created_at, updated_at
			FROM billing.subscriptions
			WHERE auto_renew = true AND status = 'active' AND expires_at <= $1
			ORDER BY expires_at ASC
			LIMIT 100;
		`
		rows, err := tx.Query(txCtx, query, now)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var sub billing.Subscription
			var statusStr string
			if err := rows.Scan(
				&sub.ID, &sub.PublicID, &sub.UserID, &sub.OrganizationID, &sub.PlanID,
				&statusStr, &sub.BillingCycle, &sub.AutoRenew,
				&sub.StartsAt, &sub.ExpiresAt, &sub.LastRenewedAt, &sub.RenewalAttempts,
				&sub.SourceSystem, &sub.SourceID, &sub.CreatedAt, &sub.UpdatedAt,
			); err != nil {
				return err
			}
			sub.Status = billing.SubscriptionStatus(statusStr)
			list = append(list, &sub)
		}
		return rows.Err()
	})
	return list, err
}

// UpdateSubscriptionStatus updates the status and retry counter of a subscription.
func (r *Repository) UpdateSubscriptionStatus(ctx context.Context, id int64, status billing.SubscriptionStatus, renewalAttempts int) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE billing.subscriptions
			SET status = $1, renewal_attempts = $2, updated_at = now()
			WHERE id = $3;
		`, string(status), renewalAttempts, id)
		return err
	})
}

// RenewSubscription performs atomic wallet deduction and advances the subscription expiration date.
func (r *Repository) RenewSubscription(ctx context.Context, subID int64, walletID int64, cost money.Amount, newExpiresAt time.Time, details string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Lock subscription row
		var currentStatus string
		var planID int64
		var orgID *int64
		var userID int64
		err := tx.QueryRow(txCtx, `
			SELECT status, plan_id, organization_id, user_id
			FROM billing.subscriptions
			WHERE id = $1
			FOR UPDATE;
		`, subID).Scan(&currentStatus, &planID, &orgID, &userID)
		if err != nil {
			return err
		}

		// 2. If cost > 0, deduct from wallet with ledger record
		if !cost.IsZero() && !cost.IsNegative() {
			var balance money.Amount
			err = tx.QueryRow(txCtx, `
				SELECT COALESCE(SUM(amount), 0)
				FROM billing.wallet_transactions
				WHERE wallet_id = $1;
			`, walletID).Scan(&balance)
			if err != nil {
				return err
			}

			pendingWithdrawals, _ := r.pendingWithdrawals(txCtx, tx, walletID)

			availMinor := balance.Minor() - pendingWithdrawals.Minor()
			if availMinor < cost.Minor() {
				return apperr.Conflict("wallet.insufficient_funds", i18n.TDefault("w4_mod.w4str_75_75"))
			}

			newBalance, _ := balance.Sub(cost)
			negCost := money.FromMinor(-cost.Minor())

			_, err = tx.Exec(txCtx, `
				INSERT INTO billing.wallet_transactions (
					wallet_id, type, amount, balance_after, reference_type, reference_id, description
				) VALUES ($1, $2, $3, $4, $5, $6, $7);
			`, walletID, string(billing.TxPurchase), negCost, newBalance, "subscription_renewal", subID, i18n.TDefault("w4_mod.w4str_76_76")+details)
			if err != nil {
				return err
			}
		}

		// 3. Update subscription expiration and renewal metadata
		_, err = tx.Exec(txCtx, `
			UPDATE billing.subscriptions
			SET expires_at = $1, last_renewed_at = now(), renewal_attempts = 0, status = 'active', updated_at = now()
			WHERE id = $2;
		`, newExpiresAt, subID)
		if err != nil {
			return err
		}

		// 4. Record history entry
		var histOrgID int64
		if orgID != nil {
			histOrgID = *orgID
		}
		_, _ = tx.Exec(txCtx, `
			INSERT INTO billing.subscription_histories (
				subscription_id, organization_id, user_id, plan_id, action, amount_minor, currency, details
			) VALUES ($1, $2, $3, $4, 'renewed', $5, 'EGP', $6);
		`, subID, histOrgID, userID, planID, cost.Minor(), details)

		return nil
	})
}

// CheckEntitlement resolves whether a user has access to a specific feature via plan.
func (r *Repository) CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error) {
	var val string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT pf.value
			FROM billing.subscriptions s
			JOIN billing.plan_features pf ON pf.plan_id = s.plan_id
			WHERE s.user_id = $1 AND s.status = 'active' AND s.expires_at > now() AND pf.feature_key = $2
			ORDER BY s.expires_at DESC
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, query, userID, featureKey).Scan(&val)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return false, "", nil
		}
		return false, "", err
	}
	return true, val, nil
}

// CheckOrgEntitlement resolves whether an organization or user has access to a specific feature key via their active subscription or default plan.
func (r *Repository) CheckOrgEntitlement(ctx context.Context, orgID, userID int64, featureKey string) (bool, error) {
	var val string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Check active subscription for organization or user
		querySub := `
			SELECT pf.value
			FROM billing.subscriptions s
			JOIN billing.plan_features pf ON pf.plan_id = s.plan_id
			WHERE ((s.organization_id IS NOT NULL AND s.organization_id = $1 AND $1 > 0)
			   OR (s.user_id = $2 AND $2 > 0))
			  AND s.status = 'active'
			  AND s.expires_at > now()
			  AND pf.feature_key = $3
			ORDER BY s.expires_at DESC
			LIMIT 1;
		`
		err := tx.QueryRow(txCtx, querySub, orgID, userID, featureKey).Scan(&val)
		if err == nil {
			return nil
		}
		if !database.IsNotFound(err) {
			return err
		}

		// 2. Fallback to default active plan features
		queryDefault := `
			SELECT pf.value
			FROM billing.plans p
			JOIN billing.plan_features pf ON pf.plan_id = p.id
			WHERE p.is_default = true AND p.is_active = true AND pf.feature_key = $1
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, queryDefault, featureKey).Scan(&val)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return val == "true" || val == "1" || val == "enabled", nil
}

// GetSetting reads a raw JSON setting string from platform_admin.system_settings.
func (r *Repository) GetSetting(ctx context.Context, key string) (string, error) {
	var val string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT value::text FROM platform_admin.system_settings WHERE key = $1 LIMIT 1;`
		return tx.QueryRow(txCtx, query, key).Scan(&val)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}
