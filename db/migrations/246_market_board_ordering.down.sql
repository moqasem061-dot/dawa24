BEGIN;

-- Restores the state migration 190 left behind: the old partial expression index
-- on the discount ordering, no upload-date column, and no trigger maintaining
-- one. The application reads f.created_at for ordering again once the previous
-- image is deployed, which is why this is a safe rollback rather than a
-- re-upload of anything.

DROP INDEX IF EXISTS compare.idx_compare_file_rows_market_sort;

CREATE INDEX IF NOT EXISTS idx_compare_file_rows_market_sort
    ON compare.file_rows (
        (CASE WHEN COALESCE(discount, 0) > 0 AND COALESCE(discount, 0) < 100
              THEN discount ELSE 0 END) DESC,
        (CASE
           WHEN COALESCE(price_after_discount, 0) > 0 THEN price_after_discount
           WHEN COALESCE(discount, 0) > 0 AND COALESCE(discount, 0) < 100
             THEN ROUND(price * (100.0 - discount) / 100.0, 2)
           ELSE price
         END) ASC
    )
    WHERE price > 0 AND COALESCE(TRIM(raw_name), '') <> '';

DROP INDEX IF EXISTS compare.idx_compare_file_rows_market_newest;

DROP TRIGGER IF EXISTS file_rows_set_uploaded_at ON compare.file_rows;
DROP FUNCTION IF EXISTS compare.file_rows_set_uploaded_at();

ALTER TABLE compare.file_rows DROP COLUMN IF EXISTS file_uploaded_at;

COMMIT;
