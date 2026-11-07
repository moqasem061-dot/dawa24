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
func (r *Repository) ActiveMember(ctx context.Context, orgID, id int64) (bool, error) {
	return r.exists(ctx, orgID, id, `SELECT EXISTS(SELECT 1 FROM org.members WHERE organization_id=$1 AND user_id=$2 AND status='active')`)
}
func (r *Repository) exists(ctx context.Context, orgID, id int64, query string) (bool, error) {
	var found bool
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error { return tx.QueryRow(ctx, query, orgID, id).Scan(&found) })
	return found, err
}
