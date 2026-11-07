-- 249_sql_console_org_credentials.down.sql
-- Restores the table-level SELECT that migration 217 had granted.

BEGIN;

DO $$
BEGIN
    IF to_regclass('org.organizations') IS NOT NULL
       AND EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_sql_console') THEN
        REVOKE ALL ON TABLE org.organizations FROM dawa24_sql_console;
        GRANT SELECT ON TABLE org.organizations TO dawa24_sql_console;
    END IF;
END $$;

COMMIT;
