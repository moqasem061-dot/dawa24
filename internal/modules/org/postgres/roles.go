package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Company roles.
//
// Every query here takes an organization id and puts it in the WHERE clause.
// That is not defence in depth, it is the boundary itself: the previous
// implementation read `WHERE id = $1` under database.AsSystem, so any signed-in
// user who guessed a role id could read, rewrite or delete another company's
// role — including granting themselves permissions through it. The application
// connects to PostgreSQL as a superuser, which makes row-level security inert,
// so nothing else was standing between one tenant's roles and another's.

const roleColumns = `id, organization_id, key, name, description, is_system, is_owner, created_at`

// ListRoles returns one company's roles, oldest first, with their permissions.
func (r *Repository) ListRoles(ctx context.Context, orgID int64) ([]*org.Role, error) {
	if orgID <= 0 {
		return nil, apperr.Validation("org.required", "An organization is required to list roles.", nil)
	}
	var list []*org.Role
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `SELECT `+roleColumns+`
			  FROM org.roles
			 WHERE organization_id = $1 AND deleted_at IS NULL
			 ORDER BY is_owner DESC, is_system DESC, created_at ASC;`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		byID := map[int64]*org.Role{}
		for rows.Next() {
			role, err := scanRole(rows)
			if err != nil {
				return err
			}
			list = append(list, role)
			byID[role.ID] = role
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(list) == 0 {
			return nil
		}

		permRows, err := tx.Query(txCtx, `
			SELECT rp.role_id, rp.permission_key
			  FROM org.role_permissions rp
			  JOIN org.roles r ON r.id = rp.role_id
			 WHERE r.organization_id = $1
			 ORDER BY rp.permission_key ASC;`, orgID)
		if err != nil {
			return err
		}
		defer permRows.Close()
		for permRows.Next() {
			var id int64
			var key string
			if err := permRows.Scan(&id, &key); err != nil {
				return err
			}
			if role, ok := byID[id]; ok {
				role.Permissions = append(role.Permissions, key)
			}
		}
		return permRows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("org postgres: list roles for organization %d: %w", orgID, err)
	}
	return list, nil
}

// GetRole reads one role, and only if it belongs to the named company.
func (r *Repository) GetRole(ctx context.Context, orgID, roleID int64) (*org.Role, error) {
	var role *org.Role
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `SELECT `+roleColumns+`
			  FROM org.roles
			 WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;`, roleID, orgID)
		var scanned org.Role
		if err := row.Scan(&scanned.ID, &scanned.OrganizationID, &scanned.Key, &scanned.Name,
			&scanned.Description, &scanned.IsSystem, &scanned.IsOwner, &scanned.CreatedAt); err != nil {
			return err
		}
		role = &scanned

		permRows, err := tx.Query(txCtx,
			`SELECT permission_key FROM org.role_permissions WHERE role_id = $1 ORDER BY permission_key ASC;`, roleID)
		if err != nil {
			return err
		}
		defer permRows.Close()
		for permRows.Next() {
			var key string
			if err := permRows.Scan(&key); err != nil {
				return err
			}
			role.Permissions = append(role.Permissions, key)
		}
		return permRows.Err()
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// A role belonging to another company is reported as missing, not
			// as forbidden: "forbidden" confirms the id exists.
			return nil, apperr.NotFound("role")
		}
		return nil, fmt.Errorf("org postgres: get role %d for organization %d: %w", roleID, orgID, err)
	}
	return role, nil
}

// CreateRole inserts a company role and its grants.
func (r *Repository) CreateRole(ctx context.Context, role *org.Role) error {
	if role == nil || role.OrganizationID <= 0 {
		return apperr.Validation("org.required", "An organization is required to create a role.", nil)
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(txCtx, `
			INSERT INTO org.roles (organization_id, key, name, description, is_system, is_owner, updated_at)
			VALUES ($1, $2, $3, $4, false, false, now())
			RETURNING id, created_at;`,
			role.OrganizationID, role.Key, role.Name, role.Description).Scan(&role.ID, &role.CreatedAt)
		if err != nil {
			return translateRoleWriteError(err)
		}
		if err := writeRolePermissions(txCtx, tx, role.ID, role.Permissions); err != nil {
			return err
		}
		return rbac.BumpVersion(txCtx, tx, rbac.OrgVersionKey(role.OrganizationID))
	})
}

// UpdateRole rewrites a role's label and grants, within its own company.
func (r *Repository) UpdateRole(ctx context.Context, orgID int64, role *org.Role) error {
	if role == nil || orgID <= 0 {
		return apperr.Validation("org.required", "An organization is required to update a role.", nil)
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE org.roles
			   SET name = $1, description = $2, updated_at = now()
			 WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL;`,
			role.Name, role.Description, role.ID, orgID)
		if err != nil {
			return translateRoleWriteError(err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("role")
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.role_permissions WHERE role_id = $1;`, role.ID); err != nil {
			return err
		}
		if err := writeRolePermissions(txCtx, tx, role.ID, role.Permissions); err != nil {
			return err
		}
		return rbac.BumpVersion(txCtx, tx, rbac.OrgVersionKey(orgID))
	})
}

// DeleteRole soft-deletes a custom role and moves its holders back to the
// company's default employee role.
//
// The members are moved rather than left pointing at a deleted role, because
// a dangling org_role_id resolves to no permissions at all: deleting a role
// would silently lock out everyone who held it, with no message explaining why.
func (r *Repository) DeleteRole(ctx context.Context, orgID, roleID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var isSystem bool
		err := tx.QueryRow(txCtx,
			`SELECT is_system FROM org.roles
			  WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;`, roleID, orgID).Scan(&isSystem)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("role")
		}
		if err != nil {
			return err
		}
		if isSystem {
			return apperr.Validation("org.role_is_system",
				i18n.TDefault("w4_mod.w4str_175_175"), nil)
		}

		if _, err := tx.Exec(txCtx, `
			UPDATE org.members
			   SET org_role_id = (
			        SELECT id FROM org.roles
			         WHERE organization_id = $2 AND key = 'org_employee' AND deleted_at IS NULL
			         LIMIT 1),
			       role_key = 'org_employee',
			       updated_at = now()
			 WHERE org_role_id = $1 AND organization_id = $2;`, roleID, orgID); err != nil {
			return err
		}

		tag, err := tx.Exec(txCtx,
			`UPDATE org.roles SET deleted_at = now(), updated_at = now()
			  WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;`, roleID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("role")
		}
		return rbac.BumpVersion(txCtx, tx, rbac.OrgVersionKey(orgID))
	})
}

// AssignMemberRole points a member at one of their own company's roles.
func (r *Repository) AssignMemberRole(ctx context.Context, orgID, memberID, roleID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var key string
		var isOwner bool
		err := tx.QueryRow(txCtx,
			`SELECT key, is_owner FROM org.roles
			  WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;`, roleID, orgID).Scan(&key, &isOwner)
		if errors.Is(err, pgx.ErrNoRows) {
			// Either the role does not exist or it belongs to another company.
			// The two are one answer here on purpose.
			return apperr.NotFound("role")
		}
		if err != nil {
			return err
		}
		if isOwner || key == "org_owner" {
			return apperr.Forbidden("cannot_assign_owner_role", "لا يمكن تعيين رتبة مالك المنشأة للموظفين")
		}
		// role_key must stay a valid identity.roles key: a custom role's key is
		// company-specific and has no platform row, so members of custom roles
		// carry the generic employee key and their authority comes from
		// org_role_id.
		platformKey := "org_employee"
		if _, isSystem := rbac.OrganizationRole(key); isSystem {
			platformKey = key
		}
		tag, err := tx.Exec(txCtx, `
			UPDATE org.members
			   SET org_role_id = $1, role_key = $2, updated_at = now()
			 WHERE id = $3 AND organization_id = $4;`, roleID, platformKey, memberID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("member")
		}
		return rbac.BumpVersion(txCtx, tx, rbac.OrgVersionKey(orgID))
	})
}

// CountRoleMembers returns how many active members hold each role, so the role
// list can warn before a change affects people.
func (r *Repository) CountRoleMembers(ctx context.Context, orgID int64) (map[int64]int, error) {
	counts := map[int64]int{}
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT r.id, count(m.id)
			  FROM org.roles r
			  LEFT JOIN org.members m
			         ON m.organization_id = r.organization_id
			        AND m.status = 'active'
			        AND COALESCE(m.org_role_id, (
			              SELECT r2.id FROM org.roles r2
			               WHERE r2.organization_id = m.organization_id
			                 AND r2.key = m.role_key AND r2.deleted_at IS NULL
			               LIMIT 1)) = r.id
			 WHERE r.organization_id = $1 AND r.deleted_at IS NULL
			 GROUP BY r.id;`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				return err
			}
			counts[id] = n
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("org postgres: count role members for organization %d: %w", orgID, err)
	}
	return counts, nil
}

func writeRolePermissions(ctx context.Context, tx pgx.Tx, roleID int64, perms []string) error {
	for _, perm := range perms {
		if perm == "" {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO org.role_permissions (role_id, permission_key)
			 VALUES ($1, $2) ON CONFLICT DO NOTHING;`, roleID, perm); err != nil {
			return fmt.Errorf("org postgres: grant %s to role %d: %w", perm, roleID, err)
		}
	}
	return nil
}

// translateRoleWriteError turns the unique-key violation into the message an
// operator can act on. Without it, naming a role after one that already exists
// surfaces as a raw constraint name.
func translateRoleWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
		return apperr.Validation("org.role_key_taken",
			i18n.TDefault("w4_mod.w4str_231_231"), nil)
	}
	return err
}

func scanRole(rows pgx.Rows) (*org.Role, error) {
	var role org.Role
	if err := rows.Scan(&role.ID, &role.OrganizationID, &role.Key, &role.Name,
		&role.Description, &role.IsSystem, &role.IsOwner, &role.CreatedAt); err != nil {
		return nil, err
	}
	return &role, nil
}
