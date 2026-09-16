-- 142_wallet_deposits_and_compare_temp_warehouse.down.sql

BEGIN;

DROP TABLE IF EXISTS billing.wallet_deposits;
ALTER TABLE compare.files DROP COLUMN IF EXISTS is_temp_warehouse;

COMMIT;
