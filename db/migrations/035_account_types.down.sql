-- 035_account_types (down)
BEGIN;

ALTER TABLE org.organizations
    DROP COLUMN IF EXISTS pharmacist_license,
    DROP COLUMN IF EXISTS branch_count;

DELETE FROM identity.role_permissions WHERE role_key IN ('org_accountant','org_warehouse');
DELETE FROM identity.roles WHERE key IN ('org_accountant','org_warehouse');

COMMIT;
