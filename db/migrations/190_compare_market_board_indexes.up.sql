-- Indexes that make خصومات السوق an index-driven page instead of three full scans.
--
-- The board rendered one page of 24 cards by reading the whole of
-- compare.file_rows three times over. Measured on the production database,
-- 121,496 rows in compare.file_rows of which 98,370 belong to a live temporary
-- warehouse, EXPLAIN (ANALYZE) execution time:
--
--   listing, with COUNT(*) OVER() ....... 145.7 ms   Seq Scan + WindowAgg over
--                                                    98,370 rows, spilling
--                                                    15.9 MB to disk
--   supplier filter dropdown ............  44.2 ms   Seq Scan of the join, to
--                                                    return 79 distinct names
--   -----------------------------------------------
--   per page view ....................... ~190 ms of database CPU
--
-- On the two vCPUs this platform's database runs on, that is about ten page
-- views a second before the database is saturated — for a page that is public,
-- unauthenticated, and reachable by any crawler.
--
-- Two of those three passes are removed in code (market_discounts.go and
-- market_board_service.go): the window aggregate becomes a separate COUNT that
-- the service caches for a minute, and the supplier list is read from
-- compare.files instead of from every row of every file — 44.2 ms to 0.97 ms,
-- best of five with both queries warm, returning the identical 79 names.
--
-- This migration supplies the indexes the rewritten listing needs. Measured by
-- applying this file inside a transaction on the production database and
-- running the SQL the repository actually generates, then rolling back:
--
--   sort mode                     before      after
--   ----------------------------------------------------
--   discount_desc (the default)   59.1 ms     0.41 ms
--   price_asc                     58.8 ms     0.34 ms
--   price_desc                    58.8 ms     0.34 ms
--   discount_asc                  60.4 ms    18.3 ms   (incremental sort)
--   newest / oldest               55.9 ms    58.1 ms   (ordered by files.created_at;
--                                                       see the note at the end)
--
-- The two indexes cost 6.7 MB on disk and built in under 300 ms on 121k rows.
--
-- Note on CONCURRENTLY: migrations here run inside a transaction (see
-- internal/platform/database/migrate.go), which forbids CREATE INDEX
-- CONCURRENTLY. A plain CREATE INDEX takes a SHARE lock — reads continue,
-- writes wait. If compare.file_rows grows past a few million rows, build these
-- by hand with CONCURRENTLY first; IF NOT EXISTS then makes this a no-op.

-- 1. The default ordering of the board, as an index.
--
-- The expressions have to match the query character for character in meaning —
-- they are the same CASE expressions the listing selects, sorts and filters by,
-- and the reason those are constants in market_discounts.go rather than
-- repeated literals is so this index cannot quietly stop matching them.
--
-- The clamp is not decoration. 6,182 of the live rows carry discount = 100.00,
-- which is a column mapped to the wrong place rather than a supplier giving
-- stock away; sorting by the raw column put all of them on page one, each
-- rendering as "0%". The board ranks by the number it prints, so the index
-- must too.
--
-- The partial predicate carries BOTH row-level conditions of the board, so a
-- row the board would never show is not in the index at all: it keeps the index
-- to the rows that matter and lets the planner satisfy the WHERE clause from
-- the index instead of rechecking the heap.
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

-- 2. The two price orderings, and the EXISTS probe behind the supplier list.
--
-- Same partial predicate, so the planner can use it for the board's own rows
-- and for "does this warehouse have any priced row at all", which is what the
-- rewritten ListDistinctSuppliers asks once per file instead of once per row.
CREATE INDEX IF NOT EXISTS idx_compare_file_rows_market_price
    ON compare.file_rows (price)
    WHERE price > 0 AND COALESCE(TRIM(raw_name), '') <> '';

-- Not indexed, deliberately (1): the pager's COUNT. A partial index on
-- (file_id) took it from 33 ms to 25 ms — counting 98,370 rows costs what it
-- costs, whichever structure they are counted in. It is cached instead, in
-- market_board_service.go, which takes it out of the page entirely rather than
-- making it 24% cheaper at the price of a third index on a table that has
-- 30,000 rows copied into it on every supplier upload.
--
-- Not indexed, deliberately (2): the "newest" and "oldest" orderings sort by
-- compare.files.created_at, a column on the other side of the join, so no index
-- on compare.file_rows can serve them — 55.9 ms before these indexes and
-- 58.1 ms after. They still improve on where they started, 145.7 ms, because
-- the window aggregate is gone. If they ever become a hot path, the fix is to
-- denormalise the warehouse's upload date onto the row, not another index here.
