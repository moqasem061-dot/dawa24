BEGIN;

DROP TABLE IF EXISTS catalog.product_clients CASCADE;
DROP TABLE IF EXISTS catalog.product_infos CASCADE;
DROP TABLE IF EXISTS catalog.product_variants CASCADE;
DROP TABLE IF EXISTS catalog.products CASCADE;
DROP TABLE IF EXISTS catalog.brands CASCADE;
DROP TABLE IF EXISTS catalog.categories CASCADE;

COMMIT;
