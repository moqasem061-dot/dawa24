-- 222_dawa24_app_role.up.sql
--
-- Provisions the dedicated non-superuser application role: dawa24_app.
--
-- When the web application connects as PostgreSQL superuser 'postgres',
-- PostgreSQL natively ignores all row-level security (RLS) policies
-- (internal/platform/database/connection.go roleBypassesRLS).
--
-- Running the application under 'dawa24_app' ensures:
--   * NOBYPASSRLS is strictly enforced by PostgreSQL engine;
--   * Table-level RLS policies using platform.tenant_visible() are active;
--   * DDL operations (DROP, ALTER, CREATE TABLE) are restricted to migration runners.
--
BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_app') THEN
        CREATE ROLE dawa24_app WITH LOGIN PASSWORD 'dawa24_app_change_me' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS;
    ELSE
        ALTER ROLE dawa24_app WITH NOBYPASSRLS NOSUPERUSER;
    END IF;
END $$;

DO $$
DECLARE
    s text;
    app_schemas text[] := ARRAY[
        'ai','assistant','billing','catalog','chat','commerce','compare','hr',
        'identity','ingest','inventory','notifications','org','platform','platform_admin',
        'profile','promo','smartorder','telegram','whatsapp','workflow','public'
    ];
BEGIN
    FOREACH s IN ARRAY app_schemas LOOP
        IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = s) THEN
            EXECUTE format('GRANT USAGE ON SCHEMA %I TO dawa24_app', s);
            EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %I TO dawa24_app', s);
            EXECUTE format('GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA %I TO dawa24_app', s);
            EXECUTE format('GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA %I TO dawa24_app', s);

            EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO dawa24_app', s);
            EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO dawa24_app', s);
            EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT EXECUTE ON FUNCTIONS TO dawa24_app', s);
        END IF;
    END LOOP;
END $$;

-- Allow dawa24_app to assume assistant and sql_console roles if needed
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_assistant_ro') THEN
        GRANT dawa24_assistant_ro TO dawa24_app;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_sql_console') THEN
        GRANT dawa24_sql_console TO dawa24_app;
    END IF;
END $$;

COMMIT;
