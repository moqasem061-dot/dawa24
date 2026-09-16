-- 116_catalog_import_source_file (down)

BEGIN;

ALTER TABLE catalog.import_sessions DROP COLUMN IF EXISTS source_file;

COMMIT;
