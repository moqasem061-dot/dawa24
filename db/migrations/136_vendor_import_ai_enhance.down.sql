-- 136_vendor_import_ai_enhance (down)

BEGIN;

ALTER TABLE ingest.catalog_imports DROP COLUMN IF EXISTS ai_stats;

COMMIT;
