package rbac

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// load reads one caller's authority from the database.
//
// Three questions, in order, because each answer decides whether the next one
// matters:
//
//  1. What platform role does this user hold, and is it staff? A staff caller
//     is answered entirely from the platform side.
//  2. What membership does this user have in the requested organization? An
//     organization member's capability comes from the membership, never from
//     their platform role (Rebuild V2 rule 1) — otherwise a pharmacy employee
//     with role 'admin' left over from a data import would hold the admin
//     dashboard.
//  3. Which permissions does that role carry, restricted to the dashboard the
//     role belongs to?
func (r *Resolver) load(ctx context.Context, userID, orgID int64) (Grant, error) {
	g := Grant{UserID: userID, OrganizationID: orgID}
	c := Default()

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := loadPlatformSide(txCtx, tx, &g); err != nil {
			return err
		}
		if orgID > 0 {
			if err := loadMembership(txCtx, tx, &g); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Grant{}, err
	}

	switch {
	case g.IsPlatformOwner:
		g.Scope = ScopeAdmin
		g.Keys = c.KeysFor(ScopeAdmin)
	case g.IsStaff:
		g.Scope = ScopeAdmin
		g.Keys = c.Restrict(g.Keys, ScopeAdmin)
	case g.IsOrgOwner:
		g.Keys = c.KeysFor(g.Scope)
	default:
		if g.Scope == "" {
			// No dashboard: a user with no membership and no staff role. They
			// keep an empty holding, which denies every gated route rather
			// than falling through to a default.
			g.Keys = nil
		} else {
			g.Keys = c.Restrict(g.Keys, g.Scope)
		}
	}
	g.Permissions = NewSet(g.Keys)
	return g, nil
}

func loadPlatformSide(ctx context.Context, tx pgx.Tx, g *Grant) error {
	var isStaff bool
	err := tx.QueryRow(ctx, `
		SELECT u.role, COALESCE(ro.is_staff, false), COALESCE(u.avatar_url, ''),
		       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), '')
		  FROM identity.users u
		  LEFT JOIN identity.roles ro
		         ON ro.key = u.role AND ro.deleted_at IS NULL
		 WHERE u.id = $1 AND u.deleted_at IS NULL AND u.status = 'active';
	`, g.UserID).Scan(&g.PlatformRole, &isStaff, &g.AvatarURL, &g.Name)
	if err == pgx.ErrNoRows {
		// A deleted or suspended account resolves to nothing. This is the
		// second half of suspension: revoking the sessions closes open tabs,
		// and this closes anything that reaches the resolver afterwards.
		return nil
	}
	if err != nil {
		return fmt.Errorf("rbac: read platform role for user %d: %w", g.UserID, err)
	}
	g.Active = true
	g.IsStaff = isStaff
	if def, ok := PlatformRole(g.PlatformRole); ok && def.Owner {
		g.IsPlatformOwner = true
		return nil
	}
	if !isStaff {
		return nil
	}

	rows, err := tx.Query(ctx, `
		SELECT rp.permission_key
		  FROM identity.role_permissions rp
		  JOIN identity.permissions p ON p.key = rp.permission_key
		 WHERE rp.role_key = $1;
	`, g.PlatformRole)
	if err != nil {
		return fmt.Errorf("rbac: read platform grants for %s: %w", g.PlatformRole, err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return err
		}
		g.Keys = append(g.Keys, k)
	}
	return rows.Err()
}

// loadMembership resolves the company role.
//
// A member's role is org_role_id when they were assigned a custom one, and
// otherwise the company's own row for their role_key. The COALESCE is what
// makes members created before this system work: they carry a role_key such
// as 'org_manager' and no org_role_id, and the company's seeded org_manager
// role is the correct answer for them.
func loadMembership(ctx context.Context, tx pgx.Tx, g *Grant) error {
	var (
		roleID    *int64
		roleKey   string
		roleName  []byte
		isOwner   bool
		orgType   string
		orgStatus string
		branchID  *int64
	)
	err := tx.QueryRow(ctx, `
		SELECT m.role_key,
		       o.type,
		       o.status,
		       -- No role row (never seeded, or soft-deleted) leaves these NULL.
		       -- That member holds nothing; it must not be a scan error, which
		       -- the browser path answers by falling back to the session's
		       -- stale permission copy.
		       r.id, r.name, COALESCE(r.is_owner, false),
		       m.branch_id
		  FROM org.members m
		  JOIN org.organizations o ON o.id = m.organization_id
		  LEFT JOIN org.roles r
		         ON r.organization_id = m.organization_id
		        AND r.deleted_at IS NULL
		        AND r.id = COALESCE(
		                m.org_role_id,
		                (SELECT r2.id FROM org.roles r2
		                  WHERE r2.organization_id = m.organization_id
		                    AND r2.key = m.role_key
		                    AND r2.deleted_at IS NULL
		                  LIMIT 1))
		 WHERE m.user_id = $1
		   AND m.organization_id = $2
		   AND m.status = 'active'
		   AND m.is_active = true
		   AND o.deleted_at IS NULL;
	`, g.UserID, g.OrganizationID).Scan(&roleKey, &orgType, &orgStatus, &roleID, &roleName, &isOwner, &branchID)
	if err == pgx.ErrNoRows {
		// Not a member of the organization they asked about. Nothing is
		// granted for it — including to a staff user, whose authority over a
		// tenant comes from platform permissions, not from a membership they
		// do not have.
		return nil
	}
	if err != nil {
		return fmt.Errorf("rbac: read membership for user %d in org %d: %w", g.UserID, g.OrganizationID, err)
	}

	// The canonical type, never the raw column. Publishing "supplier" here
	// undid the normalization login performs, and every consumer that compares
	// against "vendor" then silently took the wrong branch.
	g.OrgType = NormalizeOrgType(orgType)
	g.OrgStatus = orgStatus
	g.BranchID = branchID
	g.MemberRoleKey = roleKey
	if scope, ok := TenantScopeFor(orgType); ok {
		g.Scope = scope
	}
	if len(roleName) > 0 {
		var name map[string]string
		if err := json.Unmarshal(roleName, &name); err == nil {
			g.MemberRoleName = name["ar"]
			if g.MemberRoleName == "" {
				g.MemberRoleName = name["en"]
			}
		}
	}
	// The owner flag is taken from the seeded role, and from the legacy role
	// key as a fallback for a company whose roles have not been seeded yet.
	g.IsOrgOwner = isOwner || roleKey == "org_owner"
	if g.IsOrgOwner {
		return nil
	}

	if roleID != nil {
		rows, err := tx.Query(ctx, `
			SELECT rp.permission_key
			  FROM org.role_permissions rp
			 WHERE rp.role_id = $1;
		`, *roleID)
		if err != nil {
			return fmt.Errorf("rbac: read role grants for role %d: %w", *roleID, err)
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				return err
			}
			g.Keys = append(g.Keys, k)
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}

	// Fallback for unseeded or legacy memberships:
	// If the organization role was not found in org.roles or has no permission rows,
	// fall back to the built-in system role definition for roleKey so employees
	// never end up with 0 permissions.
	if len(g.Keys) == 0 && roleKey != "" {
		if sysRole, ok := OrganizationRole(roleKey); ok {
			g.Keys = GrantsFor(sysRole, g.Scope)
		}
	}
	return nil
}
