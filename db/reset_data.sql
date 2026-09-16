-- Dawa24 Platform Dynamic Database Reset Script
-- Safely wipes all application and business data rows while preserving reference tables (cities, roles, settings) and super_admin account.

DO $$
DECLARE
    r RECORD;
    admin_hash TEXT := '$2a$10$eB.fynqhAD5CNDFEFeosfuc2BQx4c6g701gV1z6J0/t4Y49tK6lqS'; -- Valid bcrypt hash for "Dawa24!Test"
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
        '{"ar":"مدير المنصة العام","en":"Platform Super Admin"}'::jsonb,
        'super_admin',
        'active',
        'ar',
        'Africa/Cairo'
    )
    ON CONFLICT (email) WHERE deleted_at IS NULL DO UPDATE SET
        password_hash = admin_hash,
        name          = '{"ar":"مدير المنصة العام","en":"Platform Super Admin"}'::jsonb,
        role          = 'super_admin',
        status        = 'active',
        deleted_at    = NULL,
        updated_at    = now();

    INSERT INTO identity.users (email, password_hash, name, role, status, language, timezone)
    VALUES (
        'admin@dawa24.test',
        admin_hash,
        '{"ar":"مدير المنصة العام","en":"Platform Super Admin"}'::jsonb,
        'super_admin',
        'active',
        'ar',
        'Africa/Cairo'
    )
    ON CONFLICT (email) WHERE deleted_at IS NULL DO UPDATE SET
        password_hash = admin_hash,
        name          = '{"ar":"مدير المنصة العام","en":"Platform Super Admin"}'::jsonb,
        role          = 'super_admin',
        status        = 'active',
        deleted_at    = NULL,
        updated_at    = now();

    INSERT INTO identity.user_security (user_id, login_attempts, locked_until)
    SELECT id, 0, NULL FROM identity.users WHERE email IN ('admin@dawa24.com', 'admin@dawa24.test')
    ON CONFLICT (user_id) DO UPDATE SET login_attempts = 0, locked_until = NULL;

END $$;