-- 062_offer_commerce (down)

BEGIN;

ALTER TABLE promo.offer_products
    DROP COLUMN IF EXISTS max_qty_per_order,
    DROP COLUMN IF EXISTS custom_qty,
    DROP COLUMN IF EXISTS custom_discount_amount,
    DROP COLUMN IF EXISTS custom_discount_percentage,
    DROP COLUMN IF EXISTS custom_price,
    DROP COLUMN IF EXISTS variant_id;

ALTER TABLE promo.offers
    DROP COLUMN IF EXISTS min_order_amount,
    DROP COLUMN IF EXISTS rejected_by,
    DROP COLUMN IF EXISTS rejected_at,
    DROP COLUMN IF EXISTS approved_by,
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS admin_notes,
    DROP COLUMN IF EXISTS admin_status,
    DROP COLUMN IF EXISTS branch_id;

DROP INDEX IF EXISTS promo.offers_admin_status_idx;
DROP INDEX IF EXISTS promo.offers_branch_idx;

COMMIT;