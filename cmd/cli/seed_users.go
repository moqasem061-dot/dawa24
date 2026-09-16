package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The shared password for every seeded account. Long enough for
// identity.HashPassword, which rejects anything under eight characters.
//
// These accounts are for development and staging. Seeding refuses to run when
// APP_ENV is prod, because a known password on a real platform is a back door.
const seedPassword = "Dawa24!Test"

type seedUser struct {
	email   string
	nameAr  string
	nameEn  string
	role    string // identity.users.role - platform role
	orgName string // empty for a user with no organization
	orgType string
	orgRole string // org.members.role_key
}

// runSeedUsers creates one account per persona, each with the organization and
// membership its screens need.
//
// Signing in is not enough on its own: the vendor and customer screens filter
// on the actor's organization, so an account with no membership sees empty
// lists everywhere and looks broken rather than new.
func runSeedUsers(ctx context.Context, db *database.DB, log *slog.Logger) error {
	users := []seedUser{
		{
			email:  "admin@dawa24.test",
			nameAr: i18n.TDefault("w4_mod.s_470_470"),
			nameEn: "Platform Admin",
			// super_admin bypasses the granular permission checks in
			// authctx.RequirePermission, so this account reaches every /admin/
			// screen without needing rows in the permission tables.
			role: "super_admin",
		},
		{
			email:   "vendor@dawa24.test",
			nameAr:  i18n.TDefault("w4_mod.s_471_471"),
			nameEn:  "Test Supplier",
			role:    "user",
			orgName: i18n.TDefault("w4_mod.s_472_472"),
			orgType: "vendor",
			orgRole: "org_owner",
		},
		{
			email:   "pharmacy@dawa24.test",
			nameAr:  i18n.TDefault("w4_mod.s_473_473"),
			nameEn:  "Test Pharmacy",
			role:    "user",
			orgName: i18n.TDefault("w4_mod.s_474_474"),
			orgType: "customer",
			orgRole: "org_owner",
		},
	}

	hash, err := identity.HashPassword(seedPassword)
	if err != nil {
		return fmt.Errorf("hash seed password: %w", err)
	}

	return db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		for _, u := range users {
			name, err := i18n.New(u.nameAr, u.nameEn).Value()
			if err != nil {
				return fmt.Errorf("encode name for %s: %w", u.email, err)
			}

			// Re-running resets the password and clears any suspension, so a
			// seeded account that was suspended while testing comes back.
			var userID int64
			if err := tx.QueryRow(txCtx, `
				INSERT INTO identity.users (email, password_hash, name, role, status)
				VALUES ($1, $2, $3::jsonb, $4, 'active')
				-- users_email_key is a partial unique index
				-- (WHERE deleted_at IS NULL), so the predicate has to be repeated
				-- here or PostgreSQL cannot match an arbiter for the conflict.
				ON CONFLICT (email) WHERE deleted_at IS NULL DO UPDATE SET
					password_hash = EXCLUDED.password_hash,
					name          = EXCLUDED.name,
					role          = EXCLUDED.role,
					status        = 'active',
					deleted_at    = NULL,
					updated_at    = now()
				RETURNING id;
			`, u.email, hash, name, u.role).Scan(&userID); err != nil {
				return fmt.Errorf("upsert user %s: %w", u.email, err)
			}

			if u.orgName == "" {
				log.InfoContext(ctx, "seeded user", "email", u.email, "role", u.role, "user_id", userID)
				continue
			}

			orgName, err := i18n.New(u.orgName, u.nameEn).Value()
			if err != nil {
				return fmt.Errorf("encode org name for %s: %w", u.email, err)
			}

			// legal_name is the natural key here: there is no unique constraint
			// on it, so look before inserting rather than relying on ON CONFLICT.
			var orgID int64
			err = tx.QueryRow(txCtx,
				`SELECT id FROM org.organizations WHERE legal_name = $1 LIMIT 1;`, u.orgName,
			).Scan(&orgID)
			if err != nil {
				if !database.IsNotFound(err) {
					return fmt.Errorf("look up org %s: %w", u.orgName, err)
				}
				if err := tx.QueryRow(txCtx, `
					INSERT INTO org.organizations (name, legal_name, trade_name, type, status)
					VALUES ($1::jsonb, $2, $1::jsonb, $3, 'approved')
					RETURNING id;
				`, orgName, u.orgName, u.orgType).Scan(&orgID); err != nil {
					return fmt.Errorf("create org %s: %w", u.orgName, err)
				}
			} else {
				// An existing organization may still be pending from an earlier
				// run; approved is what the screens expect.
				if _, err := tx.Exec(txCtx,
					`UPDATE org.organizations SET status = 'approved', type = $2 WHERE id = $1;`,
					orgID, u.orgType,
				); err != nil {
					return fmt.Errorf("approve org %s: %w", u.orgName, err)
				}
			}

			// The membership is what login reads to pick an active organization.
			// Without it the session carries organization 0 and every
			// tenant-scoped query returns nothing.
			// org.members has no unique constraint on (organization_id, user_id) -
			// only non-unique indexes - so the same user can be added to one
			// organization repeatedly and ON CONFLICT has no arbiter to use.
			// Update first, insert only when nothing was updated.
			tag, err := tx.Exec(txCtx, `
				UPDATE org.members SET role_key = $3, status = 'active'
				WHERE organization_id = $1 AND user_id = $2;
			`, orgID, userID, u.orgRole)
			if err != nil {
				return fmt.Errorf("update member %s: %w", u.email, err)
			}
			if tag.RowsAffected() == 0 {
				if _, err := tx.Exec(txCtx, `
					INSERT INTO org.members (organization_id, user_id, role_key, status)
					VALUES ($1, $2, $3, 'active');
				`, orgID, userID, u.orgRole); err != nil {
					return fmt.Errorf("add member %s: %w", u.email, err)
				}
			}

			log.InfoContext(ctx, "seeded user",
				"email", u.email, "role", u.role, "user_id", userID,
				"organization_id", orgID, "org_role", u.orgRole)
		}
		return nil
	})
}

// seedUsersSummary prints the credentials, since the whole point of the
// command is to hand them to someone.
func seedUsersSummary() string {
	return fmt.Sprintf(`
Development sign-in accounts (password is the same for all three):

  password: %s

  admin@dawa24.test       super_admin   /admin/dashboard
  vendor@dawa24.test      vendor        /vendor/products
  pharmacy@dawa24.test    customer      /catalog

Sign in at /auth/login. Re-running this command resets the passwords and
clears any suspension applied while testing.
`, seedPassword)
}
