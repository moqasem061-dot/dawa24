-- 064_cart_offer_link (up)
--
-- Cart lines remember which offer they were added from (Rebuild V2 §3.3).
-- Until now the offer id posted at add-to-cart was dropped and only the price
-- survived; without the link, checkout cannot carry the offer identity into
-- the order. Line-level offer_product attribution is Phase 5 (cart-per-offer);
-- this migration only carries the offer id forward.

BEGIN;

ALTER TABLE commerce.cart_items
    ADD COLUMN IF NOT EXISTS offer_id BIGINT REFERENCES promo.offers(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS cart_items_offer_idx ON commerce.cart_items (offer_id);

COMMENT ON COLUMN commerce.cart_items.offer_id IS 'العرض الذي أُضيفت منه السلعة — offer the item was added under (064)';

COMMIT;