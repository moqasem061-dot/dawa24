-- 218_performance_indexes.down.sql
BEGIN;

DROP INDEX IF EXISTS promo.idx_offer_products_product_id;
DROP INDEX IF EXISTS promo.idx_offer_products_variant_id;
DROP INDEX IF EXISTS billing.idx_billing_invoices_order_id;
DROP INDEX IF EXISTS commerce.idx_commerce_orders_user_address_id;
DROP INDEX IF EXISTS catalog.idx_catalog_variants_branch_id;
DROP INDEX IF EXISTS chat.idx_chat_participants_org_id;
DROP INDEX IF EXISTS commerce.idx_commerce_order_lines_product_id;
DROP INDEX IF EXISTS inventory.idx_inventory_stocks_variant_qty;

COMMIT;
