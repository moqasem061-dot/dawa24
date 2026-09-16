package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Platform role storage.
//
// Every write bumps identity.rbac_version in the same transaction. That is the
// mechanism that makes a revocation visible: the resolver in every process
// compares the version it cached against the current one, so an administrator
// who removes a permission and immediately asks a colleague to reload sees the
// new answer rather than the one cached at their colleague's login.

const platformRoleColumns = `key, name, COALESCE(description, ''), is_system, is_staff`

// ListPlatformRoles returns every live platform role with grants and headcount.
func (r *Repository) ListPlatformRoles(ctx context.Context) ([]*identity.PlatformRole, error) {
	var list []*identity.PlatformRole
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT `+platformRoleColumns+`,
			       (SELECT count(*) FROM identity.users u
			         WHERE u.role = ro.key AND u.deleted_at IS NULL)
			  FROM identity.roles ro
			 WHERE ro.deleted_at IS NULL AND ro.scope = 'platform'
			 ORDER BY ro.is_system DESC, ro.key ASC;`)
		if err != nil {
			return err
		}
		defer rows.Close()
		byKey := map[string]*identity.PlatformRole{}
		for rows.Next() {
			role, err := scanPlatformRole(rows)
			if err != nil {
				return err
			}
			list = append(list, role)
			byKey[role.Key] = role
		}
		if err := rows.Err(); err != nil {
			return err
		}

		permRows, err := tx.Query(txCtx, `
			SELECT role_key, permission_key FROM identity.role_permissions
			 ORDER BY permission_key ASC;`)
		if err != nil {
			return err
		}
		defer permRows.Close()
		for permRows.Next() {
			var key, perm string
			if err := permRows.Scan(&key, &perm); err != nil {
				return err
			}
			if role, ok := byKey[key]; ok {
				role.Permissions = append(role.Permissions, perm)
			}
		}
		return permRows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("identity postgres: list platform roles: %w", err)
	}
	// The owner role's holding is the whole admin catalogue and is not stored
	// row by row; reporting it as empty would make the editor show a role with
	// no permissions that can nevertheless do everything.
	for _, role := range list {
		if def, ok := rbac.PlatformRole(role.Key); ok && def.Owner {
			role.IsOwner = true
			role.Permissions = rbac.Default().KeysFor(rbac.ScopeAdmin)
		}
	}
	return list, nil
}

// GetPlatformRole reads one role with its grants.
func (r *Repository) GetPlatformRole(ctx context.Context, key string) (*identity.PlatformRole, error) {
	var role *identity.PlatformRole
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var scanned identity.PlatformRole
		var name []byte
		err := tx.QueryRow(txCtx, `
			SELECT `+platformRoleColumns+`,
			       (SELECT count(*) FROM identity.users u
			         WHERE u.role = ro.key AND u.deleted_at IS NULL)
			  FROM identity.roles ro
			 WHERE ro.key = $1 AND ro.deleted_at IS NULL AND ro.scope = 'platform';`, key).
			Scan(&scanned.Key, &name, &scanned.Description, &scanned.IsSystem, &scanned.IsStaff, &scanned.UserCount)
		if err != nil {
			return err
		}
		scanned.Name = decodeText(name)
		role = &scanned

		permRows, err := tx.Query(txCtx,
			`SELECT permission_key FROM identity.role_permissions
			  WHERE role_key = $1 ORDER BY permission_key ASC;`, key)
		if err != nil {
			return err
		}
		defer permRows.Close()
		for permRows.Next() {
			var perm string
			if err := permRows.Scan(&perm); err != nil {
				return err
			}
			role.Permissions = append(role.Permissions, perm)
		}
		return permRows.Err()
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("role")
		}
		return nil, fmt.Errorf("identity postgres: get platform role %s: %w", key, err)
	}
	if def, ok := rbac.PlatformRole(role.Key); ok && def.Owner {
		role.IsOwner = true
		role.Permissions = rbac.Default().KeysFor(rbac.ScopeAdmin)
	}
	return role, nil
}

// CreatePlatformRole inserts a role and its grants.
func (r *Repository) CreatePlatformRole(ctx context.Context, role *identity.PlatformRole, actorID int64) error {
	name, err := json.Marshal(map[string]string{"ar": role.Name["ar"], "en": role.Name["en"]})
	if err != nil {
		return fmt.Errorf("identity postgres: marshal role name: %w", err)
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var creator *int64
		if actorID > 0 {
			creator = &actorID
		}
		_, err := tx.Exec(txCtx, `
			INSERT INTO identity.roles (key, name, scope, is_system, is_staff, description, created_by, updated_at)
			VALUES ($1, $2, 'platform', false, $3, $4, $5, now());`,
			role.Key, name, role.IsStaff, role.Description, creator)
		if err != nil {
			return translatePlatformRoleWriteError(err)
		}
		if err := writePlatformGrants(txCtx, tx, role.Key, role.Permissions); err != nil {
			return err
		}
		return rbac.BumpVersion(txCtx, tx, rbac.PlatformVersionKey)
	})
}

// UpdatePlatformRole rewrites a role and replaces its grants.
func (r *Repository) UpdatePlatformRole(ctx context.Context, role *identity.PlatformRole, actorID int64) error {
	name, err := json.Marshal(map[string]string{"ar": role.Name["ar"], "en": role.Name["en"]})
	if err != nil {
		return fmt.Errorf("identity postgres: marshal role name: %w", err)
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE identity.roles
			   SET name = $1, description = $2, is_staff = $3, updated_at = now()
			 WHERE key = $4 AND deleted_at IS NULL;`,
			name, role.Description, role.IsStaff, role.Key)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("role")
		}
		if _, err := tx.Exec(txCtx,
			`DELETE FROM identity.role_permissions WHERE role_key = $1;`, role.Key); err != nil {
			return err
		}
		if err := writePlatformGrants(txCtx, tx, role.Key, role.Permissions); err != nil {
			return err
		}
		return rbac.BumpVersion(txCtx, tx, rbac.PlatformVersionKey)
	})
}

// DeletePlatformRole soft-deletes a custom role.
func (r *Repository) DeletePlatformRole(ctx context.Context, key string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE identity.roles SET deleted_at = now(), updated_at = now()
			 WHERE key = $1 AND is_system = false AND deleted_at IS NULL;`, key)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("role")
		}
		if _, err := tx.Exec(txCtx,
			`DELETE FROM identity.role_permissions WHERE role_key = $1;`, key); err != nil {
			return err
		}
		return rbac.BumpVersion(txCtx, tx, rbac.PlatformVersionKey)
	})
}

func writePlatformGrants(ctx context.Context, tx pgx.Tx, key string, perms []string) error {
	for _, perm := range perms {
		if perm == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO identity.role_permissions (role_key, permission_key)
			VALUES ($1, $2) ON CONFLICT DO NOTHING;`, key, perm); err != nil {
			return fmt.Errorf("identity postgres: grant %s to %s: %w", perm, key, err)
		}
	}
	return nil
}

func translatePlatformRoleWriteError(err error) error {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
		return apperr.Validation("identity.role_key_taken", i18n.TDefault("w4_mod.w4str_189_189"), nil)
	}
	return err
}

func scanPlatformRole(rows pgx.Rows) (*identity.PlatformRole, error) {
	var role identity.PlatformRole
	var name []byte
	if err := rows.Scan(&role.Key, &name, &role.Description,
		&role.IsSystem, &role.IsStaff, &role.UserCount); err != nil {
		return nil, err
	}
	role.Name = decodeText(name)
	return &role, nil
}

func decodeText(raw []byte) i18n.Text {
	out := i18n.Text{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}
