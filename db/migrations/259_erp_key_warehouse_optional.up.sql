BEGIN;
-- A pharmacy's key places purchase orders and reads its own orders; it never
-- writes stock, and a pharmacy has no warehouse page. The warehouse is needed
-- only by a key that may upload products, so it is optional otherwise.
-- Expand-only: every existing row keeps its warehouse, and the old code always
-- writes one.
ALTER TABLE erp.api_keys ALTER COLUMN warehouse_id DROP NOT NULL;
ALTER TABLE erp.api_keys ADD CONSTRAINT api_keys_product_sync_needs_warehouse
    CHECK (warehouse_id IS NOT NULL OR NOT ('erp.products.sync' = ANY (scopes)));
COMMENT ON COLUMN erp.api_keys.warehouse_id IS 'المستودع الذي تكتب إليه المزامنة؛ مطلوب فقط لمفتاح يرفع الأصناف';
COMMIT;
