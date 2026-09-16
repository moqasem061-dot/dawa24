package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// DefaultOrgInfoForUser returns the organization to make active at sign-in,
// together with its type and status so the session can carry them and the shell
// can route to the right dashboard without a query per request.
func (r *Repository) DefaultOrgInfoForUser(ctx context.Context, userID int64) (int64, string, string, error) {
	var orgID int64
	var orgType, orgStatus string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT o.id, o.type, o.status
			FROM org.organizations o
			LEFT JOIN org.members m ON m.organization_id = o.id AND m.user_id = $1 AND m.status = 'active'
			WHERE (m.user_id = $1 OR o.owner_id = $1)
			ORDER BY (o.owner_id = $1) DESC, m.id ASC
			LIMIT 1;
		`
		err := tx.QueryRow(txCtx, query, userID).Scan(&orgID, &orgType, &orgStatus)
		if err != nil && database.IsNotFound(err) {
			orgID = 0
			return nil
		}
		return err
	})
	return orgID, orgType, orgStatus, err
}

// UserBelongsToOrg checks whether a user has active membership in an organization.
func (r *Repository) UserBelongsToOrg(ctx context.Context, userID int64, orgID int64) (bool, error) {
	var belongs bool
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT EXISTS (
				SELECT 1 FROM org.members WHERE user_id = $1 AND organization_id = $2 AND status = 'active'
				UNION
				SELECT 1 FROM org.organizations WHERE owner_id = $1 AND id = $2
			);
		`
		return tx.QueryRow(txCtx, query, userID, orgID).Scan(&belongs)
	})
	return belongs, err
}

// ListUserOrganizations returns all organizations the user is a member of.
func (r *Repository) ListUserOrganizations(ctx context.Context, userID int64) ([]*identity.UserOrgMembership, error) {
	var list []*identity.UserOrgMembership
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT o.id, o.trade_name, o.type, o.status, m.role_key, (m.status = 'active') as is_active
			FROM org.members m
			JOIN org.organizations o ON o.id = m.organization_id
			WHERE m.user_id = $1 AND m.status = 'active'
			ORDER BY o.created_at ASC;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m identity.UserOrgMembership
			if err := rows.Scan(&m.OrganizationID, &m.OrgName, &m.OrgType, &m.OrgStatus, &m.RoleKey, &m.IsActive); err != nil {
				return err
			}
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, err
}

// GetOrgPlanLimits retrieves the active subscription's concurrent session & device limits for an organization,
// including any active temporary extra devices granted by platform administrators.
func (r *Repository) GetOrgPlanLimits(ctx context.Context, orgID int64) (maxSessions int, maxDevices int, planName string, err error) {
	maxSessions = 3
	maxDevices = 3
	planName = i18n.TDefault("w4_mod.s_378_378")
	if planName == "" {
		planName = "الباقة الأساسية"
	}
	if r == nil || r.db == nil {
		return maxSessions, maxDevices, planName, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	err = r.db.InReadTx(database.AsSystem(queryCtx), func(txCtx context.Context, tx pgx.Tx) error {
		foundPlan := false
		if orgID > 0 {
			// 1. Try active subscription for this organization
			querySub := `
				SELECT COALESCE(p.max_login_sessions, 3), COALESCE(p.max_devices, 3), COALESCE(p.name->>'ar', 'باقة اشتراك')
				FROM billing.subscriptions s
				JOIN billing.plans p ON s.plan_id = p.id
				WHERE s.organization_id = $1 AND s.status = 'active' AND s.expires_at > now()
				ORDER BY s.starts_at DESC, s.id DESC
				LIMIT 1;
			`
			var sMax, dMax int
			var pName string
			if qErr := tx.QueryRow(txCtx, querySub, orgID).Scan(&sMax, &dMax, &pName); qErr == nil {
				if sMax > 0 {
					maxSessions = sMax
				}
				if dMax > 0 {
					maxDevices = dMax
				}
				if pName != "" {
					planName = pName
				}
				foundPlan = true
			}
		}

		// 2. Fallback to system default plan if no active subscription
		if !foundPlan {
			queryDef := `
				SELECT COALESCE(max_login_sessions, 3), COALESCE(max_devices, 3), COALESCE(name->>'ar', 'الباقة الأساسية')
				FROM billing.plans
				WHERE is_default = true AND is_active = true
				ORDER BY id ASC
				LIMIT 1;
			`
			var sMax, dMax int
			var pName string
			if qErr := tx.QueryRow(txCtx, queryDef).Scan(&sMax, &dMax, &pName); qErr == nil {
				if sMax > 0 {
					maxSessions = sMax
				}
				if dMax > 0 {
					maxDevices = dMax
				}
				if pName != "" {
					planName = pName
				}
			}
		}

		// 3. Apply active temporary extra devices granted to this organization
		if orgID > 0 {
			queryExtra := `
				SELECT COALESCE(extra_devices, 0), extra_devices_expires_at
				FROM org.organizations
				WHERE id = $1;
			`
			var extraDevs int
			var extraExpires *time.Time
			if qErr := tx.QueryRow(txCtx, queryExtra, orgID).Scan(&extraDevs, &extraExpires); qErr == nil {
				if extraDevs > 0 && (extraExpires == nil || extraExpires.After(time.Now())) {
					maxSessions += extraDevs
					maxDevices += extraDevs
				}
			}
		}
		return nil
	})
	return maxSessions, maxDevices, planName, err
}
