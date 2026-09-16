-- Migration 121: Support Offer Variant Type and Optional Product ID on Variants
-- Enables product_variants to represent either standard catalog variants (linked to a product_id)
-- or special supplier bundle offers (variant_type = 'offer' linked to offer_id with null product_id).

BEGIN;

-- 1. Allow product_id to be nullable so offer variants don't require a master catalog product
ALTER TABLE catalog.product_variants ALTER COLUMN product_id DROP NOT NULL;

-- 2. Add variant_type column (standard vs offer)
ALTER TABLE catalog.product_variants
    ADD COLUMN IF NOT EXISTS variant_type TEXT NOT NULL DEFAULT 'standard'
    CHECK (variant_type IN ('standard', 'offer'));

-- 3. Add offer_id reference to promo.offers
ALTER TABLE catalog.product_variants
    ADD COLUMN IF NOT EXISTS offer_id BIGINT REFERENCES promo.offers(id) ON DELETE CASCADE;

-- 4. Create performance indexes
CREATE INDEX IF NOT EXISTS product_variants_variant_type_idx ON catalog.product_variants(variant_type);
CREATE INDEX IF NOT EXISTS product_variants_offer_id_idx ON catalog.product_variants(offer_id) WHERE offer_id IS NOT NULL;

COMMENT ON COLUMN catalog.product_variants.variant_type IS 'نوع الصنف: standard (صنف مفرد) أو offer (عرض خاص وباقة مجمعة)';
COMMENT ON COLUMN catalog.product_variants.offer_id IS 'معرف العرض الخاص في جدول promo.offers';

COMMIT;
