-- Migration 176: index the availability probe the catalogue now orders by.
--
-- /catalog paginates at the product level and used to order by sold_times, so
-- the eight hundred and fifty-one products a pharmacy can actually buy were
-- spread across eight hundred pages of the nineteen thousand nine hundred and
-- ninety-six in the catalogue, and page one was almost entirely placeholders
-- for products no supplier stocks. The listing now orders by "has a variant
-- with a positive quantity" first, which turns a per-row EXISTS into the
-- leading sort key.
--
-- The partial index is the whole point: only positive, undeleted rows are
-- indexed, which is the only thing the probe asks about.
BEGIN;

CREATE INDEX IF NOT EXISTS idx_stocks_variant_available
  ON inventory.stocks (product_variant_id)
  WHERE deleted_at IS NULL AND quantity > 0;

COMMENT ON INDEX inventory.idx_stocks_variant_available IS 'الأرصدة المتاحة فعلياً لكل صنف — تُستخدم في ترتيب الكتالوج وفلتر التوافر';

CREATE INDEX IF NOT EXISTS idx_product_variants_product_live
  ON catalog.product_variants (product_id)
  WHERE deleted_at IS NULL;

COMMIT;
