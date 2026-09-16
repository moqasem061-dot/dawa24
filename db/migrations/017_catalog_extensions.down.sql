BEGIN;

DROP TABLE IF EXISTS catalog.saving_products CASCADE;
DROP TABLE IF EXISTS catalog.product_alerts CASCADE;
DROP TABLE IF EXISTS catalog.customer_product_mappings CASCADE;

COMMIT;
