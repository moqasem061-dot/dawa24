-- 228_app_role_privileges.up.sql
--
-- The application truncates two tables at runtime:
--   catalog.product_index    rebuilt wholesale by the reindex job and admin reindex
--   catalog.match_decisions  cleared by the admin "clear match cache" action
--
-- Migration 222 granted dawa24_app SELECT, INSERT, UPDATE and DELETE but not
-- TRUNCATE, so under the least-privilege role both actions failed with
-- "permission denied". TRUNCATE is granted on exactly these two tables and no
-- others.
--
-- It also makes the audit trail append-only for the application, which
-- DEPLOY.md always claimed and migration 222 never did: an audit log the
-- application can rewrite is not evidence of anything. Nothing in the
-- application updates or deletes audit rows.
BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_app') THEN
        GRANT TRUNCATE ON catalog.product_index TO dawa24_app;
        GRANT TRUNCATE ON catalog.match_decisions TO dawa24_app;
        REVOKE UPDATE, DELETE, TRUNCATE ON platform.audit_log FROM dawa24_app;
    END IF;
END $$;

COMMIT;
