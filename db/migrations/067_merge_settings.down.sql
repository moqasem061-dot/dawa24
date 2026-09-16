-- 067_merge_settings (down)
--
-- Recreate platform.settings and move the rows back. value_type cannot be
-- reconstructed (system_settings never stored it) — restored rows get
-- value_type = 'json', which is the only type the JSONB value can honestly be.

BEGIN;

CREATE TABLE platform.settings (
    key         TEXT PRIMARY KEY,
    value       JSONB NOT NULL,
    value_type  TEXT NOT NULL CHECK (value_type IN ('string','integer','decimal','boolean','json','i18n')),
    description TEXT,
    is_public   BOOLEAN NOT NULL DEFAULT false,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by  BIGINT
);

COMMENT ON COLUMN platform.settings.is_public IS 'Whether the setting is safe to expose publicly';

INSERT INTO platform.settings (key, value, value_type, description, is_public, updated_at)
SELECT key, value, 'json', description, is_public, updated_at
FROM platform_admin.system_settings;

COMMIT;
