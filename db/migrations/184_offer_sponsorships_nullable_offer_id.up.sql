-- Allow product-level sponsorships in promo.offer_sponsorships where item_type = 'product'
ALTER TABLE promo.offer_sponsorships ALTER COLUMN offer_id DROP NOT NULL;
