-- 086_order_rating.down.sql
ALTER TABLE commerce.order_lines
    DROP COLUMN IF EXISTS rating;

ALTER TABLE commerce.orders
    DROP COLUMN IF EXISTS rating,
    DROP COLUMN IF EXISTS review,
    DROP COLUMN IF EXISTS rated_at,
    DROP COLUMN IF EXISTS delivered_at;
