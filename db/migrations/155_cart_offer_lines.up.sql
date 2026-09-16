-- 155_cart_offer_lines
--
-- An offer is sold as one unit at its own price. The products attached to it
-- are a manifest -- what the buyer gets for that price -- not separately
-- purchasable lines.
--
-- commerce.cart_items could not express that. product_id and product_variant_id
-- were both NOT NULL, so the only way to put an offer in a cart was to explode
-- it into its constituent products and charge for each, which is a different
-- transaction from the one the supplier published. Where an offer had no
-- manifest rows at all -- true of every offer in this database -- there was
-- nothing to explode, so "add to cart" failed on every click with a generic
-- message and no way to see why.
--
-- commerce.order_lines already allowed a null product reference and carries its
-- own product_name, so the order side has always been able to hold a line that
-- is not a catalogue item. Only the cart was in the way.
--
-- After this migration a cart line is one of two shapes, and the check
-- constraint is what keeps a third from appearing:
--
--   product line : product_id and product_variant_id set, offer_id optional
--                  (an item added *under* an offer's discount)
--   offer line   : offer_id set, both product references null

BEGIN;

ALTER TABLE commerce.cart_items
    ALTER COLUMN product_id DROP NOT NULL,
    ALTER COLUMN product_variant_id DROP NOT NULL;

ALTER TABLE commerce.cart_items
    ADD CONSTRAINT cart_items_line_shape CHECK (
        (product_id IS NOT NULL AND product_variant_id IS NOT NULL)
        OR offer_id IS NOT NULL
    );

-- The existing uniqueness is ON CONFLICT (cart_id, product_variant_id), and in
-- Postgres two NULLs are distinct, so that target can never match an offer
-- line: adding the same offer twice would insert a second row instead of
-- raising the quantity. This partial index gives offer lines their own
-- uniqueness so the upsert has something to conflict against.
CREATE UNIQUE INDEX IF NOT EXISTS cart_items_cart_offer_uniq
    ON commerce.cart_items (cart_id, offer_id)
    WHERE offer_id IS NOT NULL AND product_variant_id IS NULL;

CREATE INDEX IF NOT EXISTS cart_items_offer_idx
    ON commerce.cart_items (offer_id)
    WHERE offer_id IS NOT NULL;

COMMENT ON COLUMN commerce.cart_items.offer_id IS
    'العرض المرتبط بالسطر. عندما يكون المنتج فارغاً فالسطر هو العرض نفسه يُباع كوحدة واحدة.';

COMMIT;
