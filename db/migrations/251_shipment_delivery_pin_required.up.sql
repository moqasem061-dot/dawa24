-- 251_shipment_delivery_pin_required.up.sql
-- Delivery PIN rule for NEW shipments (batch 4 of the audit fixes).
--
-- A shipment created by this release is marked delivery_pin_required = true and
-- cannot be marked delivered without the buyer's six-digit PIN, or a staff
-- override. Shipments that already exist keep the default (false), so the ones
-- out on the road when this deploys finish the old way. No backfill is needed:
-- the marker is the cutover.

BEGIN;

ALTER TABLE commerce.order_shipments
    ADD COLUMN IF NOT EXISTS delivery_pin_required BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN commerce.order_shipments.delivery_pin_required IS
    'True for shipments created under the delivery PIN rule: they can be marked delivered only with the buyer PIN (or a staff override). False for shipments that existed before the rule, which keep the old flow.';

COMMIT;
