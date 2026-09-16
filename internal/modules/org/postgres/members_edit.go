package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Editing one member, addressed by the member row rather than by the user.
//
// The team screens used to reach AddMemberDirect for an edit, which is an
// upsert built from a freshly-constructed org.Member. Any field the form did
// not carry arrived as its zero value and was written: an edit that changed a
// job title also cleared the branch, the employee code and the role.
//
// They also disagreed about what {id} meant. /customer/employees/{id}/status
// passed the org.members id, while /edit and /delete passed the user id, and
// the two templates picked whichever matched the row they happened to be
// rendering — so deleting from the team page removed the wrong person or
// nobody. Every route now takes the member id, and this is where it is
// resolved.

// GetMember reads one membership of one organization.
func (r *Repository) GetMember(ctx context.Context, orgID, memberID int64) (*org.Member, error) {
	var m org.Member
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, organization_id, user_id, branch_id,
			       COALESCE(role_id, 0), role_key, org_role_id,
			       COALESCE(employee_code, ''), COALESCE(job_title, ''),
			       base_salary, variable_salary, is_active, created_at, updated_at
			FROM org.members
			WHERE id = $1 AND organization_id = $2;`
		return tx.QueryRow(txCtx, query, memberID, orgID).Scan(
			&m.ID, &m.OrganizationID, &m.UserID, &m.BranchID,
			&m.RoleID, &m.RoleKey, &m.OrgRoleID,
			&m.EmployeeCode, &m.JobTitle,
			&m.BaseSalary, &m.VariableSalary, &m.IsActive, &m.CreatedAt, &m.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("member")
		}
		return nil, err
	}
	return &m, nil
}

// UpdateMember writes only the fields the caller named.
//
// Every field is a pointer, and a nil pointer means "the form did not carry
// this, leave it alone". That is the whole difference from the upsert this
// replaces: a partial form can no longer erase what it does not mention.
func (r *Repository) UpdateMember(ctx context.Context, orgID, memberID int64, patch org.MemberPatch) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE org.members
			SET branch_id     = CASE WHEN $3::boolean THEN $4::bigint   ELSE branch_id     END,
			    role_key      = COALESCE(NULLIF($5::text, ''), role_key),
			    org_role_id   = CASE WHEN $6::boolean THEN $7::bigint   ELSE org_role_id   END,
			    role_id       = CASE WHEN $6::boolean THEN COALESCE($7::bigint, role_id) ELSE role_id END,
			    employee_code = CASE WHEN $8::boolean THEN $9::text     ELSE employee_code END,
			    job_title     = CASE WHEN $10::boolean THEN $11::text   ELSE job_title     END,
			    is_active     = CASE WHEN $12::boolean THEN $13::boolean ELSE is_active    END,
			    status        = CASE WHEN $12::boolean THEN (CASE WHEN $13::boolean THEN 'active' ELSE 'inactive' END) ELSE status END,
			    updated_at    = now()
			WHERE id = $1 AND organization_id = $2;`

		tag, err := tx.Exec(txCtx, query,
			memberID, orgID,
			patch.BranchID != nil, derefInt64(patch.BranchID),
			deref(patch.RoleKey),
			patch.OrgRoleID != nil, derefInt64(patch.OrgRoleID),
			patch.EmployeeCode != nil, deref(patch.EmployeeCode),
			patch.JobTitle != nil, deref(patch.JobTitle),
			patch.IsActive != nil, patch.IsActive != nil && *patch.IsActive,
		)
		if err != nil {
			return fmt.Errorf("org postgres: update member: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("member")
		}
		// Role, branch binding and active state all feed the resolved grant.
		return rbac.BumpVersion(txCtx, tx, rbac.OrgVersionKey(orgID))
	})
}

// CountMembersByBranch is one query for the whole branch list.
//
// The branches screen used to load every employee of the company to print a
// per-branch head count beside each card. It needed the numbers, not the
// people.
func (r *Repository) CountMembersByBranch(ctx context.Context, orgID int64) (map[int64]int, error) {
	counts := make(map[int64]int)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT COALESCE(branch_id, 0), COUNT(*)
			FROM org.members
			WHERE organization_id = $1 AND branch_id IS NOT NULL
			GROUP BY 1;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return fmt.Errorf("org postgres: count members by branch: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var branchID int64
			var n int
			if err := rows.Scan(&branchID, &n); err != nil {
				return err
			}
			counts[branchID] = n
		}
		return rows.Err()
	})
	return counts, err
}

// MemberOrganizations reports every organization a user already belongs to,
// so a company cannot silently adopt someone else's employee.
func (r *Repository) MemberOrganizations(ctx context.Context, userID int64) ([]int64, error) {
	var ids []int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx,
			`SELECT organization_id FROM org.members WHERE user_id = $1;`, userID)
		if err != nil {
			return fmt.Errorf("org postgres: member organizations: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	return ids, err
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt64(p *int64) *int64 {
	if p == nil || *p <= 0 {
		return nil
	}
	return p
}
