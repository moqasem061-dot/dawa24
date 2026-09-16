-- 115_catalog_import_sessions (down)

BEGIN;

DROP TABLE IF EXISTS catalog.import_staging_rows;
DROP TABLE IF EXISTS catalog.import_sessions;

COMMIT;
