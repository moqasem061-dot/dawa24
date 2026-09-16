-- 119_catalog_import_ai_matched (down)

BEGIN;

ALTER TABLE catalog.import_sessions DROP COLUMN IF EXISTS ai_matched;

COMMIT;
