package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListMembersHolding returns the company's active members whose role grants
// one permission.
//
// The role a member holds is org_role_id when they were given a custom one and
// otherwise the company's own row for their role_key — the same COALESCE the
// permission resolver uses, and for the same reason: members created before
// custom roles existed carry only a key. Resolving it differently here would
// mean the assignment list and the route gate disagreed about who a delivery
// representative is.
//
// Owners are excluded. An owner holds every permission in their dashboard by
// definition, so including them would put the company's proprietor at the top
// of every "assign to" list on the platform; a supplier whose owner genuinely
// drives a van gives themselves a role that says so.
func (r *Repository) ListMembersHolding(ctx context.Context, orgID int64, permissionKey string) ([]*org.EmployeeView, error) {
	var list []*org.EmployeeView
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT m.id, m.organization_id, m.user_id, m.branch_id, m.role_key, m.org_role_id,
			       COALESCE(m.employee_code, ''), COALESCE(m.job_title, ''),
			       m.is_active, m.created_at, m.updated_at,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email),
			       COALESCE(u.email, ''), COALESCE(u.phone, ''), COALESCE(u.status, 'active'),
			       COALESCE(NULLIF(r.name->>'ar', ''), NULLIF(r.name->>'en', ''), m.role_key),
			       COALESCE(b.name->>'ar', b.name->>'en', '')
			  FROM org.members m
			  JOIN identity.users u
			    ON u.id = m.user_id AND u.deleted_at IS NULL AND u.status = 'active'
			  JOIN org.roles r
			    ON r.organization_id = m.organization_id
			   AND r.deleted_at IS NULL
			   AND r.id = COALESCE(
			           m.org_role_id,
			           (SELECT r2.id FROM org.roles r2
			             WHERE r2.organization_id = m.organization_id
			               AND r2.key = m.role_key
			               AND r2.deleted_at IS NULL
			             LIMIT 1))
			  LEFT JOIN org.branches b
			    ON b.id = m.branch_id AND b.organization_id = m.organization_id AND b.deleted_at IS NULL
			 WHERE m.organization_id = $1
			   AND m.status = 'active'
			   AND m.is_active = true
			   AND r.is_owner = false
			   AND EXISTS (
			       SELECT 1 FROM org.role_permissions rp
			        WHERE rp.role_id = r.id AND rp.permission_key = $2)
			 ORDER BY COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email) ASC, m.id ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID, permissionKey)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m org.Member
			var userName, userEmail, userPhone, userStatus, roleName, branchName string
			if err := rows.Scan(
				&m.ID, &m.OrganizationID, &m.UserID, &m.BranchID, &m.RoleKey, &m.OrgRoleID,
				&m.EmployeeCode, &m.JobTitle, &m.IsActive, &m.CreatedAt, &m.UpdatedAt,
				&userName, &userEmail, &userPhone, &userStatus, &roleName, &branchName,
			); err != nil {
				return err
			}
			list = append(list, &org.EmployeeView{
				Member:     &m,
				UserName:   userName,
				UserEmail:  userEmail,
				UserPhone:  userPhone,
				UserStatus: userStatus,
				RoleName:   roleName,
				BranchName: branchName,
			})
		}
		return rows.Err()
	})
	return list, err
}
