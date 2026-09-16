-- 205_terminal_deleted_status.up.sql
-- Allow 'deleted' as terminal lifecycle status for organizations and users (WO-27).
--
-- Deletion is a block, never a delete. Approving an account or organization
-- deletion request marks status = 'deleted' (terminal state) while preserving
-- all rows for accounting, audit, and referential integrity.

BEGIN;

-- Expand org.organizations status check to include 'deleted'
ALTER TABLE org.organizations DROP CONSTRAINT IF EXISTS organizations_status_check;
ALTER TABLE org.organizations ADD CONSTRAINT organizations_status_check
    CHECK (status IN ('pending','approved','rejected','suspended','deleted'));

-- Expand identity.users status check to include 'deleted'
ALTER TABLE identity.users DROP CONSTRAINT IF EXISTS users_status_check;
ALTER TABLE identity.users ADD CONSTRAINT users_status_check
    CHECK (status IN ('active','inactive','suspended','pending','deleted'));

COMMIT;
