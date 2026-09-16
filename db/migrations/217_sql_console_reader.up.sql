-- 217_sql_console_reader.up.sql
--
-- The role the admin SQL console reads as.
--
-- The console runs arbitrary SELECTs typed by a holder of
-- platform.developer.sql. As the application's superuser connection that was a
-- superuser shell: pg_read_file and friends read files on the database host,
-- and "SELECT * FROM identity.users" returned password hashes however carefully
-- the substring denylist in front of it was written, because a denylist
-- matches the words typed, not the columns returned
-- (docs/AUDIT_FULL_2026-09-12.md P0-2).
--
-- ExecuteSQL now runs each statement after SET LOCAL ROLE dawa24_sql_console
-- inside a read-only transaction. The role:
--
--   * cannot log in, is not a superuser, and holds no server-file roles, so
--     pg_read_file, pg_ls_dir and large-object reads are refused by PostgreSQL
--     itself;
--   * holds SELECT only, on every table in the application schemas;
--   * holds nothing on the tables that are credentials, and on the tables that
--     mix credentials with ordinary data holds every column except the secret;
--   * has BYPASSRLS: the console is a platform-wide diagnostic by design.
--
-- Tables added later are granted by the default privileges below. A new table
-- or column that holds a secret must be added to the exclusions here, in a new
-- migration.
BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_sql_console') THEN
        CREATE ROLE dawa24_sql_console NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT BYPASSRLS;
    END IF;
END $$;

DO $$
BEGIN
    EXECUTE format('GRANT dawa24_sql_console TO %I', current_user);
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_app') THEN
        GRANT dawa24_sql_console TO dawa24_app;
    END IF;
END $$;

DO $$
DECLARE
    s text;
    app_schemas text[] := ARRAY['ai','assistant','billing','catalog','chat','commerce','compare','hr',
        'identity','ingest','inventory','notifications','org','platform','platform_admin','profile',
        'promo','smartorder','telegram','workflow','public'];
BEGIN
    FOREACH s IN ARRAY app_schemas LOOP
        IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = s) THEN
            EXECUTE format('GRANT USAGE ON SCHEMA %I TO dawa24_sql_console', s);
            EXECUTE format('GRANT SELECT ON ALL TABLES IN SCHEMA %I TO dawa24_sql_console', s);
            EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT SELECT ON TABLES TO dawa24_sql_console', s);
        END IF;
    END LOOP;
END $$;

-- Credentials, whole tables: stored settings (gateway and AI keys), MFA
-- secrets and recovery codes, Telegram link tokens, and export files (the
-- download token's hash and another user's file contents).
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['platform_admin.system_settings','identity.user_mfa','telegram.link_tokens','assistant.exports'] LOOP
        IF to_regclass(t) IS NOT NULL THEN
            EXECUTE format('REVOKE ALL ON TABLE %s FROM dawa24_sql_console', t);
        END IF;
    END LOOP;
END $$;

-- Credentials inside ordinary tables: every column except the secret.
DO $$
DECLARE
    spec record;
    cols text;
BEGIN
    FOR spec IN
        SELECT * FROM (VALUES
            ('identity', 'users', ARRAY['password_hash']),
            ('billing', 'wallet_withdrawals', ARRAY['destination_details']),
            ('compare', 'user_sessions', ARRAY['session_id'])
        ) AS v(sch, tbl, secret)
    LOOP
        IF to_regclass(format('%I.%I', spec.sch, spec.tbl)) IS NULL THEN
            CONTINUE;
        END IF;
        EXECUTE format('REVOKE ALL ON TABLE %I.%I FROM dawa24_sql_console', spec.sch, spec.tbl);
        SELECT string_agg(quote_ident(column_name), ', ' ORDER BY ordinal_position) INTO cols
          FROM information_schema.columns
         WHERE table_schema = spec.sch AND table_name = spec.tbl
           AND column_name <> ALL (spec.secret);
        EXECUTE format('GRANT SELECT (%s) ON %I.%I TO dawa24_sql_console', cols, spec.sch, spec.tbl);
    END LOOP;
END $$;

COMMIT;
