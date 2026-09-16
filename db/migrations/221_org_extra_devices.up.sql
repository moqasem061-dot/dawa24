-- 221_org_extra_devices.up.sql
-- Add temporary extra connected devices to organizations

ALTER TABLE org.organizations
    ADD COLUMN IF NOT EXISTS extra_devices INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS extra_devices_expires_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS organizations_extra_devices_idx
    ON org.organizations (extra_devices, extra_devices_expires_at)
    WHERE extra_devices > 0;
