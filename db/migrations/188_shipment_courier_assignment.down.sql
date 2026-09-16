-- 188_shipment_courier_assignment (down)

BEGIN;

DROP INDEX IF EXISTS commerce.order_shipments_unassigned_idx;
DROP INDEX IF EXISTS commerce.order_shipments_courier_queue_idx;

ALTER TABLE commerce.order_shipments
    DROP COLUMN IF EXISTS courier_assigned_by,
    DROP COLUMN IF EXISTS courier_assigned_at,
    DROP COLUMN IF EXISTS courier_user_id;

COMMIT;
