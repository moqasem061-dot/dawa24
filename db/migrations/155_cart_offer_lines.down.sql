-- Reverses 155_cart_offer_lines.
--
-- Offer lines cannot exist once the product columns are mandatory again, so
-- they are removed first. They are carts, not orders: nothing is lost that the
-- buyer cannot re-add, and leaving them would make the ALTER fail.

BEGIN;

DELETE FROM commerce.cart_items WHERE product_id IS NULL OR product_variant_id IS NULL;

DROP INDEX IF EXISTS commerce.cart_items_cart_offer_uniq;
DROP INDEX IF EXISTS commerce.cart_items_offer_idx;

ALTER TABLE commerce.cart_items DROP CONSTRAINT IF EXISTS cart_items_line_shape;

ALTER TABLE commerce.cart_items
    ALTER COLUMN product_id SET NOT NULL,
    ALTER COLUMN product_variant_id SET NOT NULL;

COMMIT;
