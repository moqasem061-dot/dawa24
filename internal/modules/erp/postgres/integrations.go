package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func (r *Repository) EnsureIntegration(ctx context.Context, orgID int64, provider string, createdBy int64, audit erp.AuditRecord) (erp.Integration, error) {
	var result erp.Integration
	err := r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id, provider, created_by) VALUES ($1,$2,NULLIF($3,0)) ON CONFLICT (organization_id, provider) WHERE status <> 'closed' DO NOTHING RETURNING id, public_id::text, organization_id, provider, status, created_by, created_at, updated_at`, orgID, provider, createdBy).Scan(&result.ID, &result.PublicID, &result.OrganizationID, &result.Provider, &result.Status, &result.CreatedBy, &result.CreatedAt, &result.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.QueryRow(ctx, `SELECT id, public_id::text, organization_id, provider, status, created_by, created_at, updated_at FROM erp.integrations WHERE organization_id=$1 AND provider=$2 AND status <> 'closed'`, orgID, provider).Scan(&result.ID, &result.PublicID, &result.OrganizationID, &result.Provider, &result.Status, &result.CreatedBy, &result.CreatedAt, &result.UpdatedAt)
		}
		if err != nil {
			return err
		}
		return writeAudit(ctx, tx, orgID, audit)
	})
	return result, err
}

// IntegrationStatus is the status of the organization's open integration, or
// "" when it has none yet.
func (r *Repository) IntegrationStatus(ctx context.Context, orgID int64) (string, error) {
	var status string
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT status FROM erp.integrations WHERE organization_id=$1 AND status <> 'closed' ORDER BY id DESC LIMIT 1`, orgID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return status, err
}

func (r *Repository) SetIntegrationStatus(ctx context.Context, orgID, integrationID int64, status string, audit erp.AuditRecord) error {
	return r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE erp.integrations SET status=$3 WHERE organization_id=$1 AND id=$2`, orgID, integrationID, status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return erp.ErrNotFound
		}
		return writeAudit(ctx, tx, orgID, audit)
	})
}
func (r *Repository) OrgStatusType(ctx context.Context, orgID int64) (status, orgType string, err error) {
	err = r.db.QueryRowUnscoped(ctx, `SELECT status, type FROM org.organizations WHERE id=$1`, orgID).Scan(&status, &orgType)
	return
}
func (r *Repository) WarehouseInOrg(ctx context.Context, orgID, id int64) (bool, error) {
	return r.exists(ctx, orgID, id, `SELECT EXISTS(SELECT 1 FROM inventory.warehouses WHERE organization_id=$1 AND id=$2 AND deleted_at IS NULL)`)
}
func (r *Repository) BranchInOrg(ctx context.Context, orgID, id int64) (bool, error) {
	return r.exists(ctx, orgID, id, `SELECT EXISTS(SELECT 1 FROM org.branches WHERE organization_id=$1 AND id=$2 AND deleted_at IS NULL)`)
}

// KeyTargets reads the organization's warehouses, branches and active members
// under its own tenant context, the same scoping WarehouseInOrg, BranchInOrg
// and ActiveMember use to check the submitted choice.
func (r *Repository) KeyTargets(ctx context.Context, orgID int64) (erp.KeyTargetSet, error) {
	var set erp.KeyTargetSet
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		var qerr error
		if set.Warehouses, qerr = scanKeyTargets(ctx, tx, `SELECT id, jsonb_build_object('ar', name, 'en', name) FROM inventory.warehouses WHERE organization_id=$1 AND deleted_at IS NULL ORDER BY id`, orgID); qerr != nil {
			return qerr
		}
		if set.Branches, qerr = scanKeyTargets(ctx, tx, `SELECT id, name FROM org.branches WHERE organization_id=$1 AND deleted_at IS NULL ORDER BY is_main DESC, id`, orgID); qerr != nil {
			return qerr
		}
		set.Members, qerr = scanKeyTargets(ctx, tx, `
			SELECT u.id, jsonb_build_object(
			           'ar', COALESCE(NULLIF(btrim(u.name->>'ar'), ''), NULLIF(btrim(u.name->>'en'), ''), fallback),
			           'en', COALESCE(NULLIF(btrim(u.name->>'en'), ''), NULLIF(btrim(u.name->>'ar'), ''), fallback))
			  FROM org.members m
			  JOIN identity.users u ON u.id = m.user_id
			 CROSS JOIN LATERAL (SELECT COALESCE(NULLIF(btrim(concat_ws(' ', u.first_name, u.last_name)), ''), u.email, '#' || u.id::text) AS fallback) l
			 WHERE m.organization_id=$1 AND m.status='active'
			 ORDER BY u.id`, orgID)
		return qerr
	})
	return set, err
}

func scanKeyTargets(ctx context.Context, tx pgx.Tx, query string, orgID int64) ([]erp.KeyTarget, error) {
	rows, err := tx.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []erp.KeyTarget
	for rows.Next() {
		var t erp.KeyTarget
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (r *Repository) ActiveMember(ctx context.Context, orgID, id int64) (bool, error) {
	return r.exists(ctx, orgID, id, `SELECT EXISTS(SELECT 1 FROM org.members WHERE organization_id=$1 AND user_id=$2 AND status='active')`)
}
func (r *Repository) exists(ctx context.Context, orgID, id int64, query string) (bool, error) {
	var found bool
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error { return tx.QueryRow(ctx, query, orgID, id).Scan(&found) })
	return found, err
}
