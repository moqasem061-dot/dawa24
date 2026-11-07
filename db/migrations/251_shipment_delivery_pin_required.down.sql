-- 251_shipment_delivery_pin_required.down.sql

BEGIN;

ALTER TABLE commerce.order_shipments DROP COLUMN IF EXISTS delivery_pin_required;

COMMIT;
