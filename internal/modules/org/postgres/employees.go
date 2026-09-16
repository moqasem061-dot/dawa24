package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListEmployees returns comprehensive employee rows with user, role, and branch details.
func (r *Repository) ListEmployees(ctx context.Context, orgID int64) ([]*org.EmployeeView, error) {
	var list []*org.EmployeeView
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT m.id, m.organization_id, m.user_id, m.branch_id, m.role_id, m.role_key,
			       m.org_role_id, COALESCE(m.employee_code, ''), COALESCE(m.job_title, ''),
			       m.base_salary, m.variable_salary, m.is_active, m.created_at, m.updated_at,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email),
			       COALESCE(u.email, ''), COALESCE(u.phone, ''), COALESCE(u.status, 'active'),
			       COALESCE(u.avatar_url, ''),
			       COALESCE(NULLIF(r.name->>'ar', ''), NULLIF(r.name->>'en', ''), NULLIF(ir.name->>'ar', ''), NULLIF(ir.name->>'en', ''), m.role_key),
			       COALESCE(b.name->>'ar', b.name->>'en', ''),
			       CASE WHEN b.manager_id = m.user_id THEN true ELSE false END AS is_manager
			FROM org.members m
			JOIN identity.users u ON u.id = m.user_id
			LEFT JOIN org.roles r ON r.id = m.org_role_id
			LEFT JOIN identity.roles ir ON ir.key = m.role_key
			LEFT JOIN org.branches b ON b.id = m.branch_id AND b.deleted_at IS NULL AND b.organization_id = m.organization_id
			WHERE m.organization_id = $1
			ORDER BY m.id DESC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m org.Member
			var roleID *int64
			var userName, userEmail, userPhone, userStatus, userAvatar, roleName, branchName string
			var isManager bool

			if err := rows.Scan(
				&m.ID, &m.OrganizationID, &m.UserID, &m.BranchID, &roleID, &m.RoleKey,
				&m.OrgRoleID, &m.EmployeeCode, &m.JobTitle,
				&m.BaseSalary, &m.VariableSalary, &m.IsActive, &m.CreatedAt, &m.UpdatedAt,
				&userName, &userEmail, &userPhone, &userStatus, &userAvatar,
				&roleName, &branchName, &isManager,
			); err != nil {
				return err
			}
			if roleID != nil {
				m.RoleID = *roleID
			}

			list = append(list, &org.EmployeeView{
				Member:     &m,
				UserName:   userName,
				UserEmail:  userEmail,
				UserPhone:  userPhone,
				UserStatus: userStatus,
				UserAvatar: userAvatar,
				RoleName:   roleName,
				BranchName: branchName,
				IsManager:  isManager,
			})
		}
		return rows.Err()
	})
	return list, err
}

// ListEmployeesWithTotal returns paginated employee rows with user, role, and branch details along with total count.
func (r *Repository) ListEmployeesWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*org.EmployeeView, int, error) {
	var list []*org.EmployeeView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const countQuery = `SELECT count(*) FROM org.members WHERE organization_id = $1;`
		if err := tx.QueryRow(txCtx, countQuery, orgID).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		const query = `
			SELECT m.id, m.organization_id, m.user_id, m.branch_id, m.role_id, m.role_key,
			       m.org_role_id, COALESCE(m.employee_code, ''), COALESCE(m.job_title, ''),
			       m.base_salary, m.variable_salary, m.is_active, m.created_at, m.updated_at,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email),
			       COALESCE(u.email, ''), COALESCE(u.phone, ''), COALESCE(u.status, 'active'),
			       COALESCE(u.avatar_url, ''),
			       COALESCE(NULLIF(r.name->>'ar', ''), NULLIF(r.name->>'en', ''), NULLIF(ir.name->>'ar', ''), NULLIF(ir.name->>'en', ''), m.role_key),
			       COALESCE(b.name->>'ar', b.name->>'en', ''),
			       CASE WHEN b.manager_id = m.user_id THEN true ELSE false END AS is_manager
			FROM org.members m
			JOIN identity.users u ON u.id = m.user_id
			LEFT JOIN org.roles r ON r.id = m.org_role_id
			LEFT JOIN identity.roles ir ON ir.key = m.role_key
			LEFT JOIN org.branches b ON b.id = m.branch_id AND b.deleted_at IS NULL AND b.organization_id = m.organization_id
			WHERE m.organization_id = $1
			ORDER BY m.id DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m org.Member
			var roleID *int64
			var userName, userEmail, userPhone, userStatus, userAvatar, roleName, branchName string
			var isManager bool

			if err := rows.Scan(
				&m.ID, &m.OrganizationID, &m.UserID, &m.BranchID, &roleID, &m.RoleKey,
				&m.OrgRoleID, &m.EmployeeCode, &m.JobTitle,
				&m.BaseSalary, &m.VariableSalary, &m.IsActive, &m.CreatedAt, &m.UpdatedAt,
				&userName, &userEmail, &userPhone, &userStatus, &userAvatar,
				&roleName, &branchName, &isManager,
			); err != nil {
				return err
			}
			if roleID != nil {
				m.RoleID = *roleID
			}

			list = append(list, &org.EmployeeView{
				Member:     &m,
				UserName:   userName,
				UserEmail:  userEmail,
				UserPhone:  userPhone,
				UserStatus: userStatus,
				UserAvatar: userAvatar,
				RoleName:   roleName,
				BranchName: branchName,
				IsManager:  isManager,
			})
		}
		return rows.Err()
	})
	return list, total, err
}
