BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_app') THEN
        REVOKE TRUNCATE ON catalog.product_index FROM dawa24_app;
        REVOKE TRUNCATE ON catalog.match_decisions FROM dawa24_app;
        GRANT UPDATE, DELETE ON platform.audit_log TO dawa24_app;
    END IF;
END $$;

COMMIT;
