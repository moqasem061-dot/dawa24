BEGIN;
ALTER TABLE erp.api_keys DROP CONSTRAINT IF EXISTS api_keys_product_sync_needs_warehouse;
-- Restoring NOT NULL fails while a pharmacy key without a warehouse exists;
-- revoke and delete such keys first.
ALTER TABLE erp.api_keys ALTER COLUMN warehouse_id SET NOT NULL;
COMMENT ON COLUMN erp.api_keys.warehouse_id IS 'المستودع الذي تكتب إليه المزامنة';
COMMIT;
