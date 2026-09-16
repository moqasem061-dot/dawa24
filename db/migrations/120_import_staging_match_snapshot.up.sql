-- 120_import_staging_match_snapshot (up)
--
-- catalog.import_staging_rows.matched_product_id records a decision, not a
-- relationship: "when this file was read, this row corresponded to that
-- product". The foreign key treated it as a live reference, which fails the
-- whole COPY if the product disappears between matching a nine-thousand-row
-- file and staging it — a window of seconds, but a real one, and the failure
-- takes the entire import with it.
--
-- Nothing depends on the constraint. The commit re-validates the id at the
-- moment it writes (the UPDATE carries "WHERE id = $1 AND deleted_at IS NULL"),
-- so a product removed in the meantime is simply not updated, which is the
-- correct outcome and already the one an admin sees in the review table.

BEGIN;

ALTER TABLE catalog.import_staging_rows
    DROP CONSTRAINT IF EXISTS import_staging_rows_matched_product_id_fkey;

COMMENT ON COLUMN catalog.import_staging_rows.matched_product_id IS
    'الصنف المطابق وقت التحليل — لقطة قرار وليست علاقة؛ يُعاد التحقق منها عند الحفظ';

COMMIT;
