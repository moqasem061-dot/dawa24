-- 086_order_rating.up.sql
-- Add order customer rating and review storage with per-line rating support (Laravel adv_orders/orders parity).

ALTER TABLE commerce.orders
    ADD COLUMN IF NOT EXISTS rating NUMERIC(3,2),
    ADD COLUMN IF NOT EXISTS review TEXT,
    ADD COLUMN IF NOT EXISTS rated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ;

COMMENT ON COLUMN commerce.orders.rating IS 'التقييم الإجمالي للطلب (متوسط معايير التقييم الثلاثة)';
COMMENT ON COLUMN commerce.orders.review IS 'ملاحظات وتعليق العميل على الطلب';
COMMENT ON COLUMN commerce.orders.rated_at IS 'تاريخ ووقت تسجيل التقييم من العميل';
COMMENT ON COLUMN commerce.orders.delivered_at IS 'تاريخ ووقت اكتمال تسليم الطلب النهائي';

ALTER TABLE commerce.order_lines
    ADD COLUMN IF NOT EXISTS rating NUMERIC(3,2);

COMMENT ON COLUMN commerce.order_lines.rating IS 'تقييم العميل للبند / الصنف الفردي (Laravel adv_orders.rating parity)';
