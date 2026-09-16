-- 129_vendor_import_staging_review (down)

BEGIN;

ALTER TABLE ingest.catalog_import_rows
    DROP COLUMN IF EXISTS custom_variant_name,
    DROP COLUMN IF EXISTS is_excluded,
    DROP COLUMN IF EXISTS is_manually_matched;

ALTER TABLE ingest.catalog_imports DROP CONSTRAINT IF EXISTS catalog_imports_phase_check;
ALTER TABLE ingest.catalog_imports ADD CONSTRAINT catalog_imports_phase_check
    CHECK (phase IN ('mapping','settings','confirm','processing','completed','failed','cancelled'));

COMMIT;