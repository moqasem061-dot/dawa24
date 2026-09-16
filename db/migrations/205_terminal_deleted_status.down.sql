-- 205_terminal_deleted_status.down.sql
BEGIN;

ALTER TABLE org.organizations DROP CONSTRAINT IF EXISTS organizations_status_check;
ALTER TABLE org.organizations ADD CONSTRAINT organizations_status_check
    CHECK (status IN ('pending','approved','rejected','suspended'));

ALTER TABLE identity.users DROP CONSTRAINT IF EXISTS users_status_check;
ALTER TABLE identity.users ADD CONSTRAINT users_status_check
    CHECK (status IN ('active','inactive','suspended','pending'));

COMMIT;
