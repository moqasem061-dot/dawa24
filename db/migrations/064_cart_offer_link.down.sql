-- 064_cart_offer_link (down)

BEGIN;

DROP INDEX IF EXISTS commerce.cart_items_offer_idx;

ALTER TABLE commerce.cart_items
    DROP COLUMN IF EXISTS offer_id;

COMMIT;