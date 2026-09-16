BEGIN;

DROP TABLE IF EXISTS inventory.temp_warehouses CASCADE;
DROP TABLE IF EXISTS inventory.warehouse_transfers CASCADE;
DROP TABLE IF EXISTS inventory.stock_movements CASCADE;
DROP TABLE IF EXISTS inventory.stocks CASCADE;
DROP TABLE IF EXISTS inventory.warehouses CASCADE;

COMMIT;
