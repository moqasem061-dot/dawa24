-- 067_merge_settings (up)
--
-- Rebuild V2 §2.4a — platform.settings duplicates platform_admin.system_settings
-- (same key/value JSONB shape; the old one adds a value_type column nobody
-- reads). No Go code reads platform.settings, so this is a straight data move.

BEGIN;

INSERT INTO platform_admin.system_settings (key, value, description, is_public, updated_at)
SELECT key, value, description, is_public, updated_at
FROM platform.settings
ON CONFLICT (key) DO NOTHING;

DROP TABLE platform.settings;

COMMIT;
