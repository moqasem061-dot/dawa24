-- Migration 153: Dedicated Courier Delivery System & Delivery Code (OTP) Verification
-- Adds delivery verification PIN code, attempt counters, lockout timestamps, and tracking indexes to commerce.order_shipments.

ALTER TABLE commerce.order_shipments
    ADD COLUMN IF NOT EXISTS delivery_code VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS delivery_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS delivery_locked_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivery_notes TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS collected_amount_minor BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS delivered_by_courier_at TIMESTAMPTZ;

-- Index for fast tracking number lookup by courier
CREATE INDEX IF NOT EXISTS order_shipments_tracking_number_idx
    ON commerce.order_shipments (tracking_number)
    WHERE tracking_number <> '';

-- Backfill delivery_code for any existing shipments that do not have one
UPDATE commerce.order_shipments
SET delivery_code = LPAD(FLOOR(100000 + RANDOM() * 900000)::TEXT, 6, '0')
WHERE delivery_code = '' OR delivery_code IS NULL;
