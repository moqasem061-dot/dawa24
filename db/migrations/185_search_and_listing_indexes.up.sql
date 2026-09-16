-- Indexes that make catalogue search and catalogue listing index-driven.
--
-- Before this, catalog.product_index carried a GIN full-text index and a GIN
-- trigram index that were NEVER used: pg_stat_user_indexes reported idx_scan = 0
-- for both, on 8.8 MB and 4.7 MB of index respectively. The reason was the
-- query, not the indexes -- SearchProductIndex OR-ed nine predicates together,
-- several of them unindexable, and one unindexable branch in an OR forces a
-- sequential scan of the whole table however good the other branches are.
--
-- Measured on this database, searching "panadol" against 19,996 rows:
--
--   old query, nine-branch OR .................... 780 ms  (Seq Scan, 2810 buffer reads)
--   all-indexable OR, no matches (worst case) ..... 5.4 ms  (BitmapOr over four GIN scans)
--   all-indexable OR, 462 matches ................ 17.7 ms
--
-- The query change is in internal/modules/catalog/postgres/index.go. This
-- migration supplies the one index that change still needed.
--
-- Note on CONCURRENTLY: migrations here run inside a transaction (see
-- internal/platform/database/migrate.go), which forbids CREATE INDEX
-- CONCURRENTLY. A plain CREATE INDEX takes a SHARE lock -- reads continue,
-- writes wait -- and on a table of this size that is about a second. If these
-- tables grow past a few million rows, build the index by hand with
-- CONCURRENTLY first; the IF NOT EXISTS below then makes this migration a
-- no-op rather than a conflict.

-- 1. search_text was the one branch of the search that had no index.
--
-- search_simple covers the Arabic name, the English name and the SKU.
-- search_text additionally covers scientific_name, pharmacology,
-- manufacturing_companies and the supplier's own name -- so dropping the branch
-- to get an index-driven plan would have silently narrowed what a pharmacist
-- can find. Indexing it keeps the recall AND the plan.
CREATE INDEX IF NOT EXISTS idx_product_index_search_text_trgm
    ON catalog.product_index USING gin (search_text gin_trgm_ops);

-- 2. Orderable products first, without a correlated subquery.
--
-- The listing ordered by a three-table EXISTS(...) evaluated per product, which
-- is what drove 45.8 million rows through sequential scans of catalog.products.
-- product_index already carries stock_quantity, so the same ordering is a plain
-- index on a boolean expression.
CREATE INDEX IF NOT EXISTS idx_product_index_stock_first
    ON catalog.product_index ((stock_quantity > 0) DESC, price_after_discount ASC)
    WHERE status = 'active';

-- 3. The two branches of the catalog.products search that had no index.
--
-- The rest of that query's predicates were already covered: products_name_ar_trgm_idx,
-- idx_products_name_en_trgm, idx_products_scientific_trgm and idx_products_active_trgm
-- all exist and all match the expressions the query uses. They were never
-- reached, because one unindexable branch in an OR sinks the whole plan.
--
-- These two close the gap, so the predicate is answerable end to end:
-- 584 ms -> 3.3 ms on a term matching nothing, planned as a BitmapOr over
-- eight index scans.
--
-- Both are partial on deleted_at IS NULL, matching the sibling indexes and the
-- query, so neither carries rows the search can never return.
CREATE INDEX IF NOT EXISTS idx_products_manufacturer_trgm
    ON catalog.products USING gin (manufacturing_companies gin_trgm_ops)
    WHERE deleted_at IS NULL AND manufacturing_companies <> '';

-- The vowel-stripped Arabic name. Egyptian price lists spell ا, و and ي
-- inconsistently, so the search compares names with them removed; without this
-- index that comparison had to be computed for every row in the table.
-- normalize_arabic and regexp_replace are both IMMUTABLE, so the expression is
-- indexable.
CREATE INDEX IF NOT EXISTS idx_products_name_ar_devowel_trgm
    ON catalog.products USING gin (
        regexp_replace(platform.normalize_arabic(name->>'ar'), '[اوي]', '', 'g') gin_trgm_ops)
    WHERE deleted_at IS NULL;

-- 4. Two foreign keys with no index. Small tables, but an unindexed FK turns
--    every delete of a referenced row into a sequential scan of the referencing
--    one, and costs nothing to close.
CREATE INDEX IF NOT EXISTS idx_managed_pages_created_by
    ON platform_admin.managed_pages (created_by);
CREATE INDEX IF NOT EXISTS idx_managed_pages_updated_by
    ON platform_admin.managed_pages (updated_by);
