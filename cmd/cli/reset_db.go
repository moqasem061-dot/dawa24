package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// runResetDB wipes all transactional, catalogue, tenant, and customer rows from
// the database while preserving the platform super_admin account and essential
// reference tables (cities, roles, review criteria).
func runResetDB(ctx context.Context, db *database.DB, log *slog.Logger) error {
	log.InfoContext(ctx, "starting database dynamic reset (leaving only super_admin)")

	adminHash, err := identity.HashPassword("Dawa24!Test")
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	plpgsql := `
DO $$
DECLARE
    r RECORD;
    admin_hash TEXT := $1;
BEGIN
    -- 1. Truncate all tables in domain schemas (except system seeds)
    FOR r IN (
        SELECT schemaname, tablename 
        FROM pg_tables 
        WHERE schemaname IN ('commerce', 'inventory', 'catalog', 'promo', 'hr', 'chat', 'workflow', 'notifications', 'billing', 'org', 'ingest')
          AND tablename NOT IN ('review_criteria', 'roles', 'role_permissions', 'feature_flags')
    ) LOOP
        EXECUTE format('TRUNCATE TABLE %I.%I CASCADE;', r.schemaname, r.tablename);
    END LOOP;

    -- 2. Truncate platform_admin operational tables (preserving seed reference data)
    FOR r IN (
        SELECT schemaname, tablename 
        FROM pg_tables 
        WHERE schemaname = 'platform_admin'
          AND tablename NOT IN ('countries', 'cities', 'system_settings', 'feature_flags', 'currencies', 'languages', 'translations', 'policies', 'content_blocks')
    ) LOOP
        EXECUTE format('TRUNCATE TABLE %I.%I CASCADE;', r.schemaname, r.tablename);
    END LOOP;

    -- 3. Truncate identity session/activity tables
    FOR r IN (
        SELECT schemaname, tablename 
        FROM pg_tables 
        WHERE schemaname = 'identity'
          AND tablename IN ('sessions', 'user_addresses', 'user_address_histories', 'user_favorites', 'user_preferences', 'audit_logs')
    ) LOOP
        EXECUTE format('TRUNCATE TABLE %I.%I CASCADE;', r.schemaname, r.tablename);
    END LOOP;

    -- 4. Delete non-admin users
    DELETE FROM identity.user_security WHERE user_id IN (SELECT id FROM identity.users WHERE role != 'super_admin');
    DELETE FROM identity.user_mfa WHERE user_id IN (SELECT id FROM identity.users WHERE role != 'super_admin');
    DELETE FROM identity.users WHERE role != 'super_admin';

    -- 5. Upsert Super Admin accounts (admin@dawa24.com and admin@dawa24.test)
    INSERT INTO identity.users (email, password_hash, name, role, status, language, timezone)
    VALUES (
        'admin@dawa24.com',
        admin_hash,
        '{"ar":i18n.TDefault("w4_mod.s_456_456"),"en":"Platform Super Admin"}'::jsonb,
        'super_admin',
        'active',
        'ar',
        'Africa/Cairo'
    )
    ON CONFLICT (email) WHERE deleted_at IS NULL DO UPDATE SET
        password_hash = admin_hash,
        name          = '{"ar":i18n.TDefault("w4_mod.s_456_456"),"en":"Platform Super Admin"}'::jsonb,
        role          = 'super_admin',
        status        = 'active',
        deleted_at    = NULL,
        updated_at    = now();

    INSERT INTO identity.users (email, password_hash, name, role, status, language, timezone)
    VALUES (
        'admin@dawa24.test',
        admin_hash,
        '{"ar":i18n.TDefault("w4_mod.s_456_456"),"en":"Platform Super Admin"}'::jsonb,
        'super_admin',
        'active',
        'ar',
        'Africa/Cairo'
    )
    ON CONFLICT (email) WHERE deleted_at IS NULL DO UPDATE SET
        password_hash = admin_hash,
        name          = '{"ar":i18n.TDefault("w4_mod.s_456_456"),"en":"Platform Super Admin"}'::jsonb,
        role          = 'super_admin',
        status        = 'active',
        deleted_at    = NULL,
        updated_at    = now();

    INSERT INTO identity.user_security (user_id, login_attempts, locked_until)
    SELECT id, 0, NULL FROM identity.users WHERE email IN ('admin@dawa24.com', 'admin@dawa24.test')
    ON CONFLICT (user_id) DO UPDATE SET login_attempts = 0, locked_until = NULL;

END $$;
`

	err = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, plpgsql, adminHash); err != nil {
			return fmt.Errorf("execute reset plpgsql block: %w", err)
		}
		log.InfoContext(txCtx, "database successfully reset to clean zero state with super_admin intact")
		return nil
	})

	return err
}

func resetDBHelp() string {
	return `
======================================================
 Database Reset Completed Successfully!
======================================================
 All application data rows have been wiped.
 The platform is now at clean zero state for testing.

 Super Admin Account Credentials:
   Email:    admin@dawa24.com (or admin@dawa24.test)
   Password: Dawa24!Test
   Role:     super_admin
   URL:      /admin/dashboard
======================================================
`
}
