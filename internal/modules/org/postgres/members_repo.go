package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ToggleMemberStatus toggles a member's active state.
func (r *Repository) ToggleMemberStatus(ctx context.Context, orgID, memberID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.members SET is_active = NOT is_active, updated_at = now() WHERE id = $1 AND organization_id = $2;`
		tag, err := tx.Exec(txCtx, query, memberID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("member")
		}
		// Deactivating a member revokes everything the membership granted.
		return rbac.BumpVersion(txCtx, tx, rbac.OrgVersionKey(orgID))
	})
}

// GetMemberByID returns one membership row scoped to its organization. The
// vendor team screen addresses employees by membership id (as toggle and role
// assignment already do); edit and delete resolve the underlying user id
// through this.
func (r *Repository) GetMemberByID(ctx context.Context, orgID, memberID int64) (*org.Member, error) {
	var m org.Member
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var roleID, orgRoleID, branchID *int64
		query := `SELECT id, organization_id, user_id, branch_id, role_id, role_key, org_role_id,
		                 COALESCE(employee_code, ''), COALESCE(job_title, ''), is_active, created_at, updated_at
		          FROM org.members WHERE id = $1 AND organization_id = $2;`
		err := tx.QueryRow(txCtx, query, memberID, orgID).Scan(
			&m.ID, &m.OrganizationID, &m.UserID, &branchID, &roleID, &m.RoleKey, &orgRoleID,
			&m.EmployeeCode, &m.JobTitle, &m.IsActive, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("member")
			}
			return err
		}
		m.BranchID = branchID
		m.OrgRoleID = orgRoleID
		if roleID != nil {
			m.RoleID = *roleID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMembersByOrg returns members of an organization.
func (r *Repository) ListMembersByOrg(ctx context.Context, orgID int64) ([]*org.Member, error) {
	var list []*org.Member
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, organization_id, user_id, role_id, role_key, is_active, created_at, updated_at FROM org.members WHERE organization_id = $1 ORDER BY id DESC;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m org.Member
			var roleID *int64
			if err := rows.Scan(&m.ID, &m.OrganizationID, &m.UserID, &roleID, &m.RoleKey, &m.IsActive, &m.CreatedAt, &m.UpdatedAt); err != nil {
				return err
			}
			if roleID != nil {
				m.RoleID = *roleID
			}
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, err
}

// RemoveMember removes a user from an organization.
func (r *Repository) RemoveMember(ctx context.Context, orgID, userID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM org.members WHERE organization_id = $1 AND user_id = $2;`
		if _, err := tx.Exec(txCtx, query, orgID, userID); err != nil {
			return err
		}
		return rbac.BumpVersion(txCtx, tx, rbac.OrgVersionKey(orgID))
	})
}
