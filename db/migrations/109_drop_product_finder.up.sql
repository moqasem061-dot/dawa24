-- 109_drop_product_finder.up.sql
-- Remove product finder tables completely from catalog schema.

BEGIN;

DROP TABLE IF EXISTS catalog.finder_options CASCADE;
DROP TABLE IF EXISTS catalog.finder_results CASCADE;
DROP TABLE IF EXISTS catalog.finder_questions CASCADE;

COMMIT;
