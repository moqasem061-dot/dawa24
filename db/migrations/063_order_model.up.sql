-- 063_order_model (up)
--
-- Rebuild V2 §3.3 — orders match Laravel's main_orders / adv_orders:
--   * one order belongs to one offer (offer_id, vendor_branch_id),
--   * it is placed for one customer branch (branch_id) to a saved address,
--   * final_price/total_discount reproduce the invoice after the offer changes
--     (order_lines carry offer_product_id + original price/discount snapshots),
--   * the status enum matches Laravel's 13 values exactly.
--
-- The legacy 'ready_for_pickup' status does not exist in Laravel; rows in
-- that state are mapped to 'out_for_delivery' (its closest equivalent).

BEGIN;

-- 1. Status enum parity (Laravel: pending, processing, confirmed, on_hold,
--    shipped, in_transit, out_for_delivery, delivered, completed, cancelled,
--    failed, returned, refunded).
ALTER TABLE commerce.orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE commerce.order_shipments DROP CONSTRAINT IF EXISTS order_shipments_status_check;

UPDATE commerce.orders          SET status = 'out_for_delivery' WHERE status = 'ready_for_pickup';
UPDATE commerce.order_shipments SET status = 'out_for_delivery' WHERE status = 'ready_for_pickup';

ALTER TABLE commerce.orders ADD CONSTRAINT orders_status_check
    CHECK (status IN ('pending','processing','confirmed','on_hold','shipped',
                      'in_transit','out_for_delivery','delivered','completed',
                      'cancelled','failed','returned','refunded'));

ALTER TABLE commerce.order_shipments ADD CONSTRAINT order_shipments_status_check
    CHECK (status IN ('pending','processing','confirmed','on_hold','shipped',
                      'in_transit','out_for_delivery','delivered','completed',
                      'cancelled','failed','returned','refunded'));


-- 2. Order-to-offer model.
ALTER TABLE commerce.orders
    ADD COLUMN IF NOT EXISTS offer_id         BIGINT REFERENCES promo.offers(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS branch_id        BIGINT REFERENCES org.branches(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS vendor_branch_id BIGINT REFERENCES org.branches(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS user_address_id  BIGINT REFERENCES identity.user_address_histories(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS total_discount   NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    ADD COLUMN IF NOT EXISTS final_price      NUMERIC(12,2) NOT NULL DEFAULT 0.00;

-- 3. Line-level price history.
ALTER TABLE commerce.order_lines
    ADD COLUMN IF NOT EXISTS offer_product_id    BIGINT REFERENCES promo.offer_products(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS original_price      NUMERIC(12,2),
    ADD COLUMN IF NOT EXISTS original_discount   NUMERIC(12,2),
    ADD COLUMN IF NOT EXISTS list_price          NUMERIC(12,2);

CREATE INDEX IF NOT EXISTS orders_offer_idx        ON commerce.orders (offer_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS orders_branch_idx       ON commerce.orders (branch_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS orders_vendor_branch_idx ON commerce.orders (vendor_branch_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS order_lines_offer_product_idx ON commerce.order_lines (offer_product_id);

COMMENT ON COLUMN commerce.orders.offer_id IS 'العرض الذي أُنشئ الطلب عليه — an order belongs to exactly one offer';
COMMENT ON COLUMN commerce.orders.branch_id IS 'فرع الصيدلية المشترية — customer branch the order is placed for';
COMMENT ON COLUMN commerce.orders.vendor_branch_id IS 'فرع المورد المُنفِّذ — the fulfilling vendor branch';
COMMENT ON COLUMN commerce.orders.total_discount IS 'إجمالي الخصم — offer discounts summed over lines';
COMMENT ON COLUMN commerce.orders.final_price IS 'السعر النهائي المدفوع بعد الخصم — total_amount minus total_discount';
COMMENT ON COLUMN commerce.order_lines.offer_product_id IS 'سطر العرض الذي بيع بموجبه الصنف — offer_product line (063)';
COMMENT ON COLUMN commerce.order_lines.original_price IS 'السعر الأصلي وقت البيع — legacy price snapshot';
COMMENT ON COLUMN commerce.order_lines.original_discount IS 'الخصم الأصلي وقت البيع';
COMMENT ON COLUMN commerce.order_lines.list_price IS 'سعر القائمة قبل الخصم — for the invoice strike-through';

COMMIT;