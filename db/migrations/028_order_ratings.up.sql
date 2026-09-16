-- 028_order_ratings
--
-- commerce.orders gained a rating endpoint before it had anywhere to put the
-- answer. POST /orders/{id}/rate writes rating, review and rated_at, none of
-- which existed, so every call would have failed with `column does not exist`.
--
-- Found by extending test/schema_consistency_test.go to INSERT and UPDATE
-- column lists, having originally covered only SELECT.

BEGIN;

ALTER TABLE commerce.orders
    ADD COLUMN IF NOT EXISTS rating   SMALLINT,
    ADD COLUMN IF NOT EXISTS review   TEXT,
    ADD COLUMN IF NOT EXISTS rated_at TIMESTAMPTZ;

-- The service already refuses anything outside 1-5. Stating it here too means
-- a future caller that bypasses the service cannot corrupt supplier scores.
ALTER TABLE commerce.orders
    ADD CONSTRAINT orders_rating_range
    CHECK (rating IS NULL OR (rating BETWEEN 1 AND 5));

-- A review or timestamp without a rating is meaningless, and a rating without a
-- timestamp cannot be aged out of supplier scoring later.
ALTER TABLE commerce.orders
    ADD CONSTRAINT orders_rating_complete
    CHECK ((rating IS NULL AND rated_at IS NULL) OR (rating IS NOT NULL AND rated_at IS NOT NULL));

COMMENT ON COLUMN commerce.orders.rating IS 'تقييم العميل للطلب — 1 to 5, set only after delivery';

-- Supplier score aggregation reads rated orders per organization.
CREATE INDEX IF NOT EXISTS orders_rating_idx
    ON commerce.orders (organization_id, rating)
    WHERE rating IS NOT NULL;

COMMIT;
