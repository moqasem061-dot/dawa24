package rbac

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Sync writes the Go catalogue into identity.permissions and reconciles the
// platform roles that ship with it.
//
// It runs at boot, after migrations. The table is a mirror, never a second
// source: a permission that no longer exists in code is deleted here, and the
// foreign keys on identity.role_permissions and org.role_permissions cascade
// the grants away with it. That is deliberate. A role holding a key for a page
// that no longer exists is a grant nobody can audit and nothing can revoke.
//
// The whole reconciliation is one transaction. A half-synced catalogue would
// leave the role editor showing sections whose permissions had gone.
func Sync(ctx context.Context, db *database.DB) error {
	c := Default()
	return db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := syncPermissions(txCtx, tx, c); err != nil {
			return err
		}
		if err := syncPlatformRoles(txCtx, tx, c); err != nil {
			return err
		}
		_, err := tx.Exec(txCtx, `SELECT identity.bump_rbac_version('platform');`)
		return err
	})
}

func syncPermissions(ctx context.Context, tx pgx.Tx, c *Catalog) error {
	keys := make([]string, 0, len(c.Permissions()))
	for i, p := range c.Permissions() {
		name, err := json.Marshal(map[string]string{"ar": p.NameAr, "en": p.NameEn})
		if err != nil {
			return fmt.Errorf("rbac sync: marshal name for %s: %w", p.Key, err)
		}
		scopes := make([]string, 0, len(p.Scopes))
		for _, s := range p.Scopes {
			scopes = append(scopes, string(s))
		}
		// module is the first key segment. It is a legacy column kept because
		// three migrations wrote it and reports still group by it.
		module := p.Key
		for i, r := range p.Key {
			if r == '.' {
				module = p.Key[:i]
				break
			}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO identity.permissions
			    (key, name, module, description, group_key, kind, scopes, nav_key, sort_order, synced_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
			ON CONFLICT (key) DO UPDATE SET
			    name       = EXCLUDED.name,
			    module     = EXCLUDED.module,
			    description= EXCLUDED.description,
			    group_key  = EXCLUDED.group_key,
			    kind       = EXCLUDED.kind,
			    scopes     = EXCLUDED.scopes,
			    nav_key    = EXCLUDED.nav_key,
			    sort_order = EXCLUDED.sort_order,
			    synced_at  = now();
		`, p.Key, name, module, p.NameEn, p.Group, string(p.Kind), scopes, p.Nav, i)
		if err != nil {
			return fmt.Errorf("rbac sync: upsert permission %s: %w", p.Key, err)
		}
		keys = append(keys, p.Key)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM identity.permissions WHERE key <> ALL($1::text[]);`, keys); err != nil {
		return fmt.Errorf("rbac sync: prune permissions: %w", err)
	}
	return nil
}

// syncPlatformRoles keeps the shipped platform roles and their grants exactly
// as declared. Custom roles a super admin creates are left untouched — they
// are is_system = false and this function never looks at them.
func syncPlatformRoles(ctx context.Context, tx pgx.Tx, c *Catalog) error {
	for _, r := range PlatformRoles() {
		name, err := json.Marshal(map[string]string{"ar": r.NameAr, "en": r.NameEn})
		if err != nil {
			return fmt.Errorf("rbac sync: marshal role name %s: %w", r.Key, err)
		}
		scope := "platform"
		if !r.IsStaff && r.Scope != ScopeAdmin {
			scope = "platform" // identity.roles.scope only distinguishes platform from organization
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO identity.roles (key, name, scope, is_system, is_staff, description, updated_at)
			VALUES ($1, $2, $3, true, $4, $5, now())
			ON CONFLICT (key) DO UPDATE SET
			    name        = EXCLUDED.name,
			    is_system   = true,
			    is_staff    = EXCLUDED.is_staff,
			    description = EXCLUDED.description,
			    deleted_at  = NULL,
			    updated_at  = now();
		`, r.Key, name, scope, r.IsStaff, r.DescAr)
		if err != nil {
			return fmt.Errorf("rbac sync: upsert role %s: %w", r.Key, err)
		}

		grants := GrantsFor(r, ScopeAdmin)
		if _, err := tx.Exec(ctx,
			`DELETE FROM identity.role_permissions WHERE role_key = $1 AND permission_key <> ALL($2::text[]);`,
			r.Key, grants); err != nil {
			return fmt.Errorf("rbac sync: prune grants for %s: %w", r.Key, err)
		}
		for _, k := range grants {
			if _, err := tx.Exec(ctx, `
				INSERT INTO identity.role_permissions (role_key, permission_key)
				VALUES ($1, $2) ON CONFLICT DO NOTHING;`, r.Key, k); err != nil {
				return fmt.Errorf("rbac sync: grant %s to %s: %w", k, r.Key, err)
			}
		}
	}

	// The organization starter roles keep their identity.roles rows so that
	// org.members.role_key's foreign key still resolves, but they carry no
	// platform grants: an organization member's capability comes from their
	// company's own role, never from a platform-wide row. Leaving the old
	// rows in place is what let a vendor employee hold "org.branch.view",
	// an admin-dashboard permission, through their membership.
	for _, r := range OrganizationRoles() {
		name, err := json.Marshal(map[string]string{"ar": r.NameAr, "en": r.NameEn})
		if err != nil {
			return fmt.Errorf("rbac sync: marshal org role name %s: %w", r.Key, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO identity.roles (key, name, scope, is_system, is_staff, description, updated_at)
			VALUES ($1, $2, 'organization', true, false, $3, now())
			ON CONFLICT (key) DO UPDATE SET
			    name        = EXCLUDED.name,
			    scope       = 'organization',
			    is_system   = true,
			    is_staff    = false,
			    description = EXCLUDED.description,
			    deleted_at  = NULL,
			    updated_at  = now();
		`, r.Key, name, r.DescAr); err != nil {
			return fmt.Errorf("rbac sync: upsert org role %s: %w", r.Key, err)
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM identity.role_permissions WHERE role_key = $1;`, r.Key); err != nil {
			return fmt.Errorf("rbac sync: clear platform grants for %s: %w", r.Key, err)
		}
	}
	_ = c
	return nil
}
