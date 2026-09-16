package rbac

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// EnsureCompanyRoles seeds the starter roles into one company, if they are not
// there already.
//
// Every company owns its roles. org_manager in company 51 is a different row
// from org_manager in company 50, with its own permission grants that its own
// owner may edit. That is the isolation requirement stated plainly in the
// schema: a role has an organization_id, so a role created by one company can
// never be selected by another.
//
// Seeded roles are marked is_system, which stops them being deleted, but their
// permissions remain editable — an owner who wants their warehouse keeper to
// also see invoices should not have to build a role from nothing.
func EnsureCompanyRoles(ctx context.Context, db *database.DB, orgID int64, orgType string) error {
	scope, ok := TenantScopeFor(orgType)
	if !ok {
		// An organization whose type has no dashboard has nobody to seed roles
		// for. Returning nil rather than an error keeps registration for an
		// unusual type working; there is simply nothing to do.
		return nil
	}
	return db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return ensureCompanyRolesTx(txCtx, tx, orgID, scope)
	})
}

func ensureCompanyRolesTx(ctx context.Context, tx pgx.Tx, orgID int64, scope Scope) error {
	for _, r := range OrganizationRolesFor(scope) {
		name, err := json.Marshal(map[string]string{"ar": r.NameAr, "en": r.NameEn})
		if err != nil {
			return fmt.Errorf("rbac provision: marshal %s: %w", r.Key, err)
		}
		var roleID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO org.roles (organization_id, key, name, description, is_system, is_owner, updated_at)
			VALUES ($1, $2, $3, $4, true, $5, now())
			ON CONFLICT (organization_id, key) DO UPDATE SET
			    is_system  = true,
			    is_owner   = EXCLUDED.is_owner,
			    deleted_at = NULL
			RETURNING id;
		`, orgID, r.Key, name, r.DescAr, r.Owner).Scan(&roleID)
		if err != nil {
			return fmt.Errorf("rbac provision: upsert role %s for org %d: %w", r.Key, orgID, err)
		}

		// Grants are written once, at creation. Re-running this must not undo
		// an owner's edits to a starter role, so an existing role keeps
		// whatever permissions it now has.
		var existing int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM org.role_permissions WHERE role_id = $1;`, roleID).Scan(&existing); err != nil {
			return fmt.Errorf("rbac provision: count grants for role %d: %w", roleID, err)
		}
		if existing > 0 {
			continue
		}
		for _, k := range GrantsFor(r, scope) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO org.role_permissions (role_id, permission_key)
				VALUES ($1, $2) ON CONFLICT DO NOTHING;`, roleID, k); err != nil {
				return fmt.Errorf("rbac provision: grant %s to role %d: %w", k, roleID, err)
			}
		}
	}
	_, err := tx.Exec(ctx, `SELECT identity.bump_rbac_version($1);`, OrgVersionKey(orgID))
	return err
}

// SeedExistingCompanies provisions starter roles for every organization that
// has none.
//
// It runs once at boot after Sync. Companies that predate this system have
// members carrying a role_key and no roles table to resolve it against; until
// they are seeded, those members resolve to no permissions at all and their
// dashboard is an empty sidebar. Seeding is idempotent and touches only
// organizations with zero roles, so a second boot does nothing.
func SeedExistingCompanies(ctx context.Context, db *database.DB) (int, error) {
	type target struct {
		id      int64
		orgType string
	}
	var todo []target
	err := db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT o.id, o.type
			  FROM org.organizations o
			 WHERE o.deleted_at IS NULL
			   AND NOT EXISTS (SELECT 1 FROM org.roles r WHERE r.organization_id = o.id)
			 ORDER BY o.id;`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t target
			if err := rows.Scan(&t.id, &t.orgType); err != nil {
				return err
			}
			todo = append(todo, t)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, fmt.Errorf("rbac provision: list unseeded organizations: %w", err)
	}

	seeded := 0
	for _, t := range todo {
		scope, ok := TenantScopeFor(t.orgType)
		if !ok {
			continue
		}
		err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			return ensureCompanyRolesTx(txCtx, tx, t.id, scope)
		})
		if err != nil {
			return seeded, fmt.Errorf("rbac provision: seed organization %d: %w", t.id, err)
		}
		seeded++
	}
	return seeded, nil
}

// SeedMissingSystemRoles adds starter roles that a company does not have yet.
//
// SeedExistingCompanies deliberately touches only companies with *no* roles at
// all, which is right for the migration it was written for and wrong for every
// role added afterwards: مندوب توصيل was declared after every supplier on the
// platform already had its six roles, so not one of them would ever have seen
// it, and the delivery portal would have looked like a feature that does not
// exist.
//
// This closes that gap once per boot, and it is cheap because it asks the
// question the other way round — "which company is missing which role" — so a
// platform with nothing to do reads zero rows rather than upserting six roles
// per company.
func SeedMissingSystemRoles(ctx context.Context, db *database.DB) (int, error) {
	type target struct {
		id    int64
		scope Scope
	}
	var todo []target

	// One pass per dashboard, asking only about the roles that dashboard has.
	// Asking globally instead would report every pharmacy as "missing"
	// org_courier on every boot — for ever, because a pharmacy is never meant
	// to have one — and the repair would be a no-op the platform paid for at
	// each start.
	for _, scope := range []Scope{ScopeVendor, ScopePharmacy} {
		keys := make([]string, 0, 8)
		for _, r := range OrganizationRolesFor(scope) {
			keys = append(keys, r.Key)
		}
		types := TenantOrgTypes(scope)
		s := scope
		err := db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			rows, err := tx.Query(txCtx, `
				SELECT DISTINCT o.id
				  FROM org.organizations o
				  JOIN org.roles have ON have.organization_id = o.id AND have.deleted_at IS NULL
				  CROSS JOIN unnest($1::text[]) AS want(key)
				 WHERE o.deleted_at IS NULL
				   AND o.type = ANY($2)
				   AND NOT EXISTS (
				       SELECT 1 FROM org.roles r
				        WHERE r.organization_id = o.id
				          AND r.key = want.key
				          AND r.deleted_at IS NULL)
				 ORDER BY o.id;`, keys, types)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					return err
				}
				todo = append(todo, target{id: id, scope: s})
			}
			return rows.Err()
		})
		if err != nil {
			return 0, fmt.Errorf("rbac provision: list %s companies missing system roles: %w", scope, err)
		}
	}

	seeded := 0
	for _, t := range todo {
		err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			return ensureCompanyRolesTx(txCtx, tx, t.id, t.scope)
		})
		if err != nil {
			return seeded, fmt.Errorf("rbac provision: seed missing roles for organization %d: %w", t.id, err)
		}
		seeded++
	}
	return seeded, nil
}

// OrgVersionKey names one company's slot in the invalidation counter.
func OrgVersionKey(orgID int64) string {
	return fmt.Sprintf("org:%d", orgID)
}

// PlatformVersionKey names the slot covering platform roles and staff grants.
const PlatformVersionKey = "platform"

// BumpVersion invalidates cached permissions.
//
// Call it inside the same transaction as the write it describes, so that a
// role change and its invalidation commit or roll back together. A bump that
// commits without its change makes every process re-read for nothing; a change
// that commits without its bump leaves revoked access live until the cache
// expires, which is the outcome worth designing against.
func BumpVersion(ctx context.Context, tx pgx.Tx, scopeKey string) error {
	_, err := tx.Exec(ctx, `SELECT identity.bump_rbac_version($1);`, scopeKey)
	if err != nil {
		return fmt.Errorf("rbac: bump version %s: %w", scopeKey, err)
	}
	return nil
}

// RepairOutOfScopeGrants removes company role grants that do not belong to the
// company's dashboard.
//
// They arise when an organization type is reclassified — "company" was read as
// a pharmacy and is now read as a supplier, which is what identity's own
// normalisation always said. The grants are already inert, because the
// resolver restricts a holding to the company's scope before answering
// anything. What they are not is honest: the role editor counts them, so an
// owner sees a manager role with twenty-nine permissions that grant nothing.
//
// Running this on every boot rather than as a one-off migration means the next
// reclassification repairs itself too.
func RepairOutOfScopeGrants(ctx context.Context, db *database.DB) (int, error) {
	c := Default()
	repaired := 0

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT DISTINCT o.id, o.type
			  FROM org.organizations o
			  JOIN org.roles r ON r.organization_id = o.id AND r.deleted_at IS NULL
			 WHERE o.deleted_at IS NULL
			 ORDER BY o.id;`)
		if err != nil {
			return err
		}
		type company struct {
			id      int64
			orgType string
		}
		var companies []company
		for rows.Next() {
			var cmp company
			if err := rows.Scan(&cmp.id, &cmp.orgType); err != nil {
				rows.Close()
				return err
			}
			companies = append(companies, cmp)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for _, cmp := range companies {
			scope, ok := TenantScopeFor(cmp.orgType)
			if !ok {
				continue
			}
			tag, err := tx.Exec(txCtx, `
				DELETE FROM org.role_permissions rp
				 USING org.roles r
				 WHERE rp.role_id = r.id
				   AND r.organization_id = $1
				   AND rp.permission_key <> ALL($2::text[]);`, cmp.id, c.KeysFor(scope))
			if err != nil {
				return fmt.Errorf("rbac repair: organization %d: %w", cmp.id, err)
			}
			if tag.RowsAffected() == 0 {
				continue
			}
			repaired++
			// A company whose starter roles were just emptied needs them back,
			// in the right scope this time. Custom roles are left alone: their
			// permissions were chosen by an owner, and the out-of-scope ones
			// have been dropped rather than replaced.
			if err := reseedSystemRoles(txCtx, tx, cmp.id, scope); err != nil {
				return err
			}
			if err := BumpVersion(txCtx, tx, OrgVersionKey(cmp.id)); err != nil {
				return err
			}
		}
		return nil
	})
	return repaired, err
}

// reseedSystemRoles restores the starter grants for a company's system roles.
// Custom roles are untouched.
func reseedSystemRoles(ctx context.Context, tx pgx.Tx, orgID int64, scope Scope) error {
	for _, r := range OrganizationRolesFor(scope) {
		var roleID int64
		err := tx.QueryRow(ctx, `
			SELECT id FROM org.roles
			 WHERE organization_id = $1 AND key = $2 AND is_system = true AND deleted_at IS NULL;`,
			orgID, r.Key).Scan(&roleID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("rbac repair: read role %s for organization %d: %w", r.Key, orgID, err)
		}
		for _, k := range GrantsFor(r, scope) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO org.role_permissions (role_id, permission_key)
				VALUES ($1, $2) ON CONFLICT DO NOTHING;`, roleID, k); err != nil {
				return fmt.Errorf("rbac repair: grant %s to role %d: %w", k, roleID, err)
			}
		}
	}
	return nil
}
