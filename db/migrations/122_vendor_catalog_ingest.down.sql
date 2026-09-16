-- 122_vendor_catalog_ingest (down)
--
-- The outcome ledger goes first: it references the session table.

BEGIN;

DROP TABLE IF EXISTS ingest.catalog_import_rows;
DROP TABLE IF EXISTS ingest.catalog_imports;

COMMIT;
