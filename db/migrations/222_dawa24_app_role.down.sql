-- 222_dawa24_app_role.down.sql
BEGIN;

DO $$
DECLARE
    s text;
    app_schemas text[] := ARRAY[
        'ai','assistant','billing','catalog','chat','commerce','compare','hr',
        'identity','ingest','inventory','notifications','org','platform','platform_admin',
        'profile','promo','smartorder','telegram','whatsapp','workflow','public'
    ];
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_app') THEN
        FOREACH s IN ARRAY app_schemas LOOP
            IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = s) THEN
                EXECUTE format('REVOKE ALL ON ALL TABLES IN SCHEMA %I FROM dawa24_app', s);
                EXECUTE format('REVOKE ALL ON ALL SEQUENCES IN SCHEMA %I FROM dawa24_app', s);
                EXECUTE format('REVOKE ALL ON ALL FUNCTIONS IN SCHEMA %I FROM dawa24_app', s);
                EXECUTE format('REVOKE USAGE ON SCHEMA %I FROM dawa24_app', s);
            END IF;
        END LOOP;
        DROP ROLE IF EXISTS dawa24_app;
    END IF;
END $$;

COMMIT;
