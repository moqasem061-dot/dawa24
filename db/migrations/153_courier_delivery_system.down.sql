-- Migration 153 Down: Revert Dedicated Courier Delivery System

DROP INDEX IF EXISTS commerce.order_shipments_tracking_number_idx;

ALTER TABLE commerce.order_shipments
    DROP COLUMN IF EXISTS delivery_code,
    DROP COLUMN IF EXISTS delivery_attempts,
    DROP COLUMN IF EXISTS delivery_locked_until,
    DROP COLUMN IF EXISTS delivery_notes,
    DROP COLUMN IF EXISTS collected_amount_minor,
    DROP COLUMN IF EXISTS delivered_by_courier_at;
