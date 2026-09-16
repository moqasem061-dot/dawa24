-- 129_vendor_import_staging_review (up)
--
-- Adds the 'review' phase to catalog imports and staging overrides to catalog_import_rows.
-- Staging rows can now store custom variant names, manual matching flags, and exclusion toggles
-- before any live data is written to the database.

BEGIN;

-- 1. Update phase check on ingest.catalog_imports
ALTER TABLE ingest.catalog_imports DROP CONSTRAINT IF EXISTS catalog_imports_phase_check;
ALTER TABLE ingest.catalog_imports ADD CONSTRAINT catalog_imports_phase_check
    CHECK (phase IN ('mapping','settings','review','confirm','processing','completed','failed','cancelled'));

-- 2. Update outcome check on ingest.catalog_import_rows
ALTER TABLE ingest.catalog_import_rows DROP CONSTRAINT IF EXISTS catalog_import_rows_outcome_check;
ALTER TABLE ingest.catalog_import_rows ADD CONSTRAINT catalog_import_rows_outcome_check
    CHECK (outcome IN ('staged','inserted','updated','skipped','error','matched','unmatched','review'));

-- 3. Add custom variant name, exclusion and manual match columns
ALTER TABLE ingest.catalog_import_rows
    ADD COLUMN IF NOT EXISTS custom_variant_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS is_excluded BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS is_manually_matched BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS catalog_import_rows_excluded
    ON ingest.catalog_import_rows (import_id, is_excluded);

COMMIT;