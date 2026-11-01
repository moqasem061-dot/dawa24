BEGIN;

-- خصومات السوق العامة — ترتيب مدعوم بفهرس بدل فرز كل الصفوف.
--
-- The board's default ordering is "newest warehouse first", and it was ordered
-- by compare.files.created_at — a column on the other side of the join. No index
-- on compare.file_rows can satisfy that, so every page view scanned and sorted
-- every row of every temporary warehouse. Measured on a 100,000-row board on the
-- two vCPUs this platform's database runs on:
--
--   default ordering (newest) ....... 118.6 ms   Parallel Seq Scan + Hash Join
--                                                + Sort over 100,000 rows
--   supplier-vs-supplier page .......  38.0 ms   COUNT(*) over the same rows
--   ----------------------------------------------------------
--   per cold page view .............. ~157 ms of database CPU
--
-- for a screen that shows twenty-four cards.
--
-- Two changes make it index-driven:
--
-- 1. The warehouse's upload date is denormalised onto the row
--    (file_uploaded_at). It is safe to denormalise because it can never change:
--    a temporary warehouse is uploaded once, and its rows are copied into a new
--    file on every re-upload rather than edited in place. Migration 190 left a
--    note that this was the fix if "newest" ever became hot. It is the default
--    sort, so it is the hot one.
--
--    A trigger fills it, rather than the ingest code, so every writer gets it:
--    the upload worker, the ETL, a hand-inserted row in an investigation. A
--    writer that forgets a column is a silent ordering bug; a trigger cannot be
--    forgotten.
--
-- 2. idx_compare_file_rows_market_sort is rebuilt against the discount
--    expression the code actually emits today.
--
--    The index from migration 190 clamps the discount with
--    `discount > 0 AND discount < 100`. The listing clamps with
--    `discount >= 0 AND discount <= 100` (so that a genuine 100% line prints as
--    100 rather than as 0). Those are different expressions, so the index
--    stopped matching the ORDER BY when the clamp changed and was never used
--    again: sorting the board by "الأعلى خصماً" measured 108.1 ms, a full sort
--    of every row, instead of the 0.41 ms the index was built for. Nothing
--    failed — the sort is still correct, just a hundred times slower.
--
-- Measured on the same 100,000 rows after this migration:
--
--   newest, page 1 .................. 0.062 ms   index scan, LIMIT stops it
--   oldest, page 1 .................. 0.080 ms   same index, scanned backwards
--   newest, page 500 (offset 11,976)   3.131 ms
--   discount_desc ................... 1.062 ms   idx_..._market_sort
--   price_asc ....................... 0.471 ms   idx_..._market_price
--
-- Both indexes are partial on the board's own row conditions, so a row the
-- board would never show is not in them.

-- 1. The warehouse's upload date, on the row.

ALTER TABLE compare.file_rows
    ADD COLUMN IF NOT EXISTS file_uploaded_at timestamp with time zone;

COMMENT ON COLUMN compare.file_rows.file_uploaded_at IS
    'تاريخ ووقت رفع المستودع الذي جاء منه الصف (نسخة من compare.files.created_at تُملأ تلقائياً بالـ trigger وتُستخدم لترتيب خصومات السوق).';

-- Backfill. A file's created_at never changes, so this runs once and the value
-- is final; the WHERE keeps a re-run cheap and makes the statement idempotent.
UPDATE compare.file_rows r
   SET file_uploaded_at = f.created_at
  FROM compare.files f
 WHERE f.id = r.file_id
   AND r.file_uploaded_at IS NULL;

-- The trigger is the only thing that writes the column. It re-reads the parent
-- on insert and whenever a row is moved to a different file, and leaves an
-- explicitly supplied value alone otherwise.
CREATE OR REPLACE FUNCTION compare.file_rows_set_uploaded_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT'
       OR NEW.file_id IS DISTINCT FROM OLD.file_id
       OR NEW.file_uploaded_at IS NULL THEN
        SELECT f.created_at INTO NEW.file_uploaded_at
          FROM compare.files f
         WHERE f.id = NEW.file_id;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS file_rows_set_uploaded_at ON compare.file_rows;
CREATE TRIGGER file_rows_set_uploaded_at
    BEFORE INSERT OR UPDATE OF file_id ON compare.file_rows
    FOR EACH ROW EXECUTE FUNCTION compare.file_rows_set_uploaded_at();

-- 2. The orderings, as indexes.

-- "newest" is `ORDER BY file_uploaded_at DESC NULLS LAST, id DESC`; "oldest" is
-- the same index scanned backwards (`ASC NULLS FIRST, id ASC`). The NULLS
-- placement is spelled out on both sides because that is what makes the backward
-- scan an exact match rather than a sort. The trigger means NULLs should not
-- exist at all; they are ordered last rather than first so that a row written
-- around the trigger shows up at the bottom of the board, not on page one.
CREATE INDEX IF NOT EXISTS idx_compare_file_rows_market_newest
    ON compare.file_rows (file_uploaded_at DESC NULLS LAST, id DESC)
    WHERE price > 0 AND COALESCE(TRIM(raw_name), '') <> '';

-- The old expression index matched a clamp the code no longer emits.
DROP INDEX IF EXISTS compare.idx_compare_file_rows_market_sort;

CREATE INDEX idx_compare_file_rows_market_sort
    ON compare.file_rows (
        (CASE WHEN COALESCE(discount, 0) >= 0 AND COALESCE(discount, 0) <= 100
              THEN COALESCE(discount, 0) ELSE 0 END) DESC,
        (CASE
           WHEN discount = 100 THEN 0
           WHEN COALESCE(price_after_discount, 0) > 0 THEN price_after_discount
           WHEN COALESCE(discount, 0) > 0 AND COALESCE(discount, 0) <= 100
             THEN ROUND(price * (100.0 - discount) / 100.0, 2)
           ELSE price
         END) ASC
    )
    WHERE price > 0 AND COALESCE(TRIM(raw_name), '') <> '';

COMMIT;
