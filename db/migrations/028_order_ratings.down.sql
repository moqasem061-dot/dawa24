BEGIN;
DROP INDEX IF EXISTS commerce.orders_rating_idx;
ALTER TABLE commerce.orders
    DROP CONSTRAINT IF EXISTS orders_rating_complete,
    DROP CONSTRAINT IF EXISTS orders_rating_range,
    DROP COLUMN IF EXISTS rated_at,
    DROP COLUMN IF EXISTS review,
    DROP COLUMN IF EXISTS rating;
COMMIT;
