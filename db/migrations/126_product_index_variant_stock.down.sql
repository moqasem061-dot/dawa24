-- 126_product_index_variant_stock (down)

BEGIN;

DROP INDEX IF EXISTS catalog.product_index_institutional_idx;
DROP INDEX IF EXISTS catalog.product_index_variant_idx;
DROP INDEX IF EXISTS catalog.product_index_in_stock_idx;
DROP INDEX IF EXISTS catalog.product_index_product_type_idx;

COMMIT;
