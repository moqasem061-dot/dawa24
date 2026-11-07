-- 249_sql_console_org_credentials.up.sql
-- Batch 3 (S15): the SQL console role could read org.organizations.ai_virtual_key
-- and ai_user_id, the AI gateway credentials of every organization. Migration 217
-- granted SELECT on the whole table. Here the table grant is replaced by a grant on
-- every column except those two. Ordinary console queries against the other columns
-- keep working; a query that names a credential column is refused by the database.

BEGIN;

DO $$
DECLARE
    cols text;
BEGIN
    IF to_regclass('org.organizations') IS NOT NULL
       AND EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_sql_console') THEN
        REVOKE ALL ON TABLE org.organizations FROM dawa24_sql_console;
        SELECT string_agg(quote_ident(column_name), ', ' ORDER BY ordinal_position) INTO cols
          FROM information_schema.columns
         WHERE table_schema = 'org' AND table_name = 'organizations'
           AND column_name <> ALL (ARRAY['ai_virtual_key', 'ai_user_id']);
        EXECUTE format('GRANT SELECT (%s) ON org.organizations TO dawa24_sql_console', cols);
    END IF;
END $$;

COMMIT;
