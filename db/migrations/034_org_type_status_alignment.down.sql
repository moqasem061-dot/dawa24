BEGIN;

ALTER TABLE org.organizations DROP CONSTRAINT IF EXISTS organizations_type_check;
ALTER TABLE org.organizations ADD CONSTRAINT organizations_type_check
    CHECK (type IN ('supplier','company','agency'));

ALTER TABLE org.organizations DROP CONSTRAINT IF EXISTS organizations_status_check;
ALTER TABLE org.organizations ADD CONSTRAINT organizations_status_check
    CHECK (status IN ('pending','approved','rejected'));

COMMIT;
