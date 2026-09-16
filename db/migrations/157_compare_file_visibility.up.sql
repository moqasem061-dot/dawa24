-- 157_compare_file_visibility.up.sql
-- Migration 157: vendor-controlled visibility for compare files / temporary warehouses.
--
-- A compare.files row is the "parent temporary warehouse". Moderator uploads
-- carry is_temp_warehouse = TRUE and always feed the public market discounts
-- page. Vendor compare-tool uploads (organization_id IS NOT NULL) stay
-- is_temp_warehouse = FALSE so the compare tool keeps listing exactly what it
-- lists today; this column lets the vendor opt an upload into the public
-- market discounts page without changing the compare tool's own behaviour.

BEGIN;

ALTER TABLE compare.files
    ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'private'
    CHECK (visibility IN ('private', 'public'));

COMMENT ON COLUMN compare.files.visibility IS
    'ظهور ملف المورد في خصومات السوق العامة: private خاص أو public عام (يتحكم به البائع)';

CREATE INDEX IF NOT EXISTS compare_files_visibility_idx
    ON compare.files (visibility) WHERE deleted_at IS NULL;

COMMIT;
