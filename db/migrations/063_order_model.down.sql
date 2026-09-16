-- 063_order_model (down)

BEGIN;

DROP INDEX IF EXISTS commerce.order_lines_offer_product_idx;
DROP INDEX IF EXISTS commerce.orders_vendor_branch_idx;
DROP INDEX IF EXISTS commerce.orders_branch_idx;
DROP INDEX IF EXISTS commerce.orders_offer_idx;

ALTER TABLE commerce.order_lines
    DROP COLUMN IF EXISTS list_price,
    DROP COLUMN IF EXISTS original_discount,
    DROP COLUMN IF EXISTS original_price,
    DROP COLUMN IF EXISTS offer_product_id;

ALTER TABLE commerce.orders
    DROP COLUMN IF EXISTS final_price,
    DROP COLUMN IF EXISTS total_discount,
    DROP COLUMN IF EXISTS user_address_id,
    DROP COLUMN IF EXISTS vendor_branch_id,
    DROP COLUMN IF EXISTS branch_id,
    DROP COLUMN IF EXISTS offer_id;

-- Restore the legacy enum; rows that were moved to out_for_delivery go back
-- to the state the old check accepts.
UPDATE commerce.orders         SET status = 'ready_for_pickup' WHERE status = 'out_for_delivery';
UPDATE commerce.order_shipments SET status = 'ready_for_pickup' WHERE status = 'out_for_delivery';

ALTER TABLE commerce.order_shipments DROP CONSTRAINT IF EXISTS order_shipments_status_check;
ALTER TABLE commerce.order_shipments ADD CONSTRAINT order_shipments_status_check
    CHECK (status IN ('pending','confirmed','processing','ready_for_pickup','shipped','delivered','cancelled','returned'));

ALTER TABLE commerce.orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE commerce.orders ADD CONSTRAINT orders_status_check
    CHECK (status IN ('pending','confirmed','processing','ready_for_pickup','shipped','delivered','cancelled','refunded'));

COMMIT;