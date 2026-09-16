-- 218_performance_indexes.up.sql
--
-- Performance indexes on frequently joined foreign keys and hot query columns:
--
--   * promo.offer_products (product_id, product_variant_id): queried during catalogue
--     and storefront offer checks, and scanned on product/variant cascading deletes.
--   * billing.invoices (order_id): invoice lookup by order and cascade checks.
--   * commerce.orders (user_address_id): scanned on user address history cleanup.
--   * catalog.product_variants (branch_id): filtered in admin and buyer branch views.
--   * chat.participants (organization_id): filtered in multi-tenant conversation queries.
--   * commerce.order_lines (product_id): queried in reporting and scanned on product delete.
--   * inventory.stocks (product_variant_id, quantity): covering index for stock balance sums.
BEGIN;

-- promo.offer_products foreign keys
CREATE INDEX IF NOT EXISTS idx_offer_products_product_id
    ON promo.offer_products (product_id);

CREATE INDEX IF NOT EXISTS idx_offer_products_variant_id
    ON promo.offer_products (product_variant_id);

-- billing.invoices order lookup
CREATE INDEX IF NOT EXISTS idx_billing_invoices_order_id
    ON billing.invoices (order_id);

-- commerce.orders address foreign key
CREATE INDEX IF NOT EXISTS idx_commerce_orders_user_address_id
    ON commerce.orders (user_address_id);

-- catalog.product_variants branch lookup
CREATE INDEX IF NOT EXISTS idx_catalog_variants_branch_id
    ON catalog.product_variants (branch_id)
    WHERE deleted_at IS NULL;

-- chat.participants organization lookup
CREATE INDEX IF NOT EXISTS idx_chat_participants_org_id
    ON chat.participants (organization_id);

-- commerce.order_lines product foreign key
CREATE INDEX IF NOT EXISTS idx_commerce_order_lines_product_id
    ON commerce.order_lines (product_id);

-- inventory.stocks covering index for stock sums
CREATE INDEX IF NOT EXISTS idx_inventory_stocks_variant_qty
    ON inventory.stocks (product_variant_id, quantity)
    WHERE deleted_at IS NULL;

COMMIT;
