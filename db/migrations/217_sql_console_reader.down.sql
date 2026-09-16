-- 217_sql_console_reader.down.sql
BEGIN;
DO $$
DECLARE
    s text;
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_sql_console') THEN
        FOREACH s IN ARRAY ARRAY['ai','assistant','billing','catalog','chat','commerce','compare','hr',
            'identity','ingest','inventory','notifications','org','platform','platform_admin','profile',
            'promo','smartorder','telegram','workflow','public'] LOOP
            IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = s) THEN
                EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA %I REVOKE SELECT ON TABLES FROM dawa24_sql_console', s);
            END IF;
        END LOOP;
        EXECUTE 'DROP OWNED BY dawa24_sql_console';
        BEGIN
            DROP ROLE dawa24_sql_console;
        EXCEPTION WHEN dependent_objects_still_exist THEN
            RAISE NOTICE 'dawa24_sql_console still has privileges in another database; kept';
        END;
    END IF;
END $$;
COMMIT;
