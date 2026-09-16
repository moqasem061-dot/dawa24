BEGIN;

DROP TABLE IF EXISTS commerce.order_status_history CASCADE;
DROP TABLE IF EXISTS commerce.order_lines CASCADE;
DROP TABLE IF EXISTS commerce.order_shipments CASCADE;
DROP TABLE IF EXISTS commerce.orders CASCADE;
DROP TABLE IF EXISTS commerce.cart_items CASCADE;
DROP TABLE IF EXISTS commerce.carts CASCADE;

DROP SCHEMA IF EXISTS commerce CASCADE;

COMMIT;
