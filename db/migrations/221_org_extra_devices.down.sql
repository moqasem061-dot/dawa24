-- 221_org_extra_devices.down.sql
DROP INDEX IF EXISTS org.organizations_extra_devices_idx;

ALTER TABLE org.organizations
    DROP COLUMN IF EXISTS extra_devices,
    DROP COLUMN IF EXISTS extra_devices_expires_at;
