-- 188_shipment_courier_assignment (up)
--
-- The delivery representative (مندوب) becomes a real account rather than a
-- shared link. A supplier assigns one of its own employees to a shipment, and
-- that employee opens إدارة الشحنات and sees exactly the parcels assigned to
-- them, oldest assignment first.
--
-- Three columns carry the whole relationship:
--   * courier_user_id      — who is carrying it, NULL while unassigned,
--   * courier_assigned_at  — when, which is the delivery priority order,
--   * courier_assigned_by  — which supplier employee handed it over.
--
-- The courier is addressed by identity.users.id rather than by org.members.id
-- because the portal authenticates a user, and a membership row may be edited
-- or replaced while a parcel is in transit. ON DELETE SET NULL returns the
-- parcel to the unassigned pool rather than blocking the account's removal.

BEGIN;

ALTER TABLE commerce.order_shipments
    ADD COLUMN IF NOT EXISTS courier_user_id     BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS courier_assigned_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS courier_assigned_by BIGINT REFERENCES identity.users(id) ON DELETE SET NULL;

-- The courier's own queue: every read the portal makes is "my parcels, oldest
-- assignment first", so the index carries the sort as well as the filter.
CREATE INDEX IF NOT EXISTS order_shipments_courier_queue_idx
    ON commerce.order_shipments (courier_user_id, courier_assigned_at)
    WHERE courier_user_id IS NOT NULL;

-- The supplier's own view: unassigned parcels, per organization.
CREATE INDEX IF NOT EXISTS order_shipments_unassigned_idx
    ON commerce.order_shipments (organization_id, created_at DESC)
    WHERE courier_user_id IS NULL;

COMMENT ON COLUMN commerce.order_shipments.courier_user_id IS
    'مندوب التوصيل المسند إليه الطرد — the delivery representative carrying this shipment; NULL while unassigned';
COMMENT ON COLUMN commerce.order_shipments.courier_assigned_at IS
    'تاريخ إسناد الطرد للمندوب — assignment time; the courier queue is ordered by it, oldest first';
COMMENT ON COLUMN commerce.order_shipments.courier_assigned_by IS
    'موظف المورد الذي أسند الطرد — the supplier employee who made the assignment';

COMMIT;
