BEGIN;

-- 1. Extend org.organization_reviews for multi-vendor parcels and explicit reviewer org
ALTER TABLE org.organization_reviews
    ADD COLUMN IF NOT EXISTS shipment_id     BIGINT REFERENCES commerce.order_shipments(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS reviewer_org_id BIGINT REFERENCES org.organizations(id) ON DELETE CASCADE;

-- 2. Drop legacy flawed unique constraints
-- reviews_one_per_order was (user_id, order_id) which prevented reviewing multiple vendors in a multi-vendor order.
-- org_reviews_user_unique was (organization_id, user_id) which prevented reviewing the same vendor on a new order.
ALTER TABLE org.organization_reviews DROP CONSTRAINT IF EXISTS org_reviews_user_unique;
ALTER TABLE org.organization_reviews DROP CONSTRAINT IF EXISTS reviews_one_per_order;
DROP INDEX IF EXISTS org.reviews_one_per_order;
DROP INDEX IF EXISTS org.org_reviews_user_unique;

-- 3. Create correct multi-vendor order review unique constraint:
-- One review per vendor (organization_id) per order (order_id)
CREATE UNIQUE INDEX IF NOT EXISTS reviews_one_per_vendor_order
    ON org.organization_reviews (organization_id, order_id)
    WHERE order_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_org_organization_reviews_shipment_id
    ON org.organization_reviews (shipment_id)
    WHERE shipment_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_org_organization_reviews_reviewer_org
    ON org.organization_reviews (reviewer_org_id)
    WHERE reviewer_org_id IS NOT NULL;

-- 4. Clean up review criteria: ensure 3 active vendor evaluation criteria
-- rep (المندوب), quality (الخدمة), speed (السرعة)
UPDATE org.review_criteria
    SET is_active = true,
        name = '{"ar":"المندوب","en":"Sales Rep"}'::jsonb,
        sort_order = 1
    WHERE key = 'rep';

UPDATE org.review_criteria
    SET is_active = true,
        name = '{"ar":"الخدمة","en":"Service & Quality"}'::jsonb,
        sort_order = 2
    WHERE key = 'quality';

UPDATE org.review_criteria
    SET is_active = true,
        name = '{"ar":"السرعة","en":"Delivery Speed"}'::jsonb,
        sort_order = 3
    WHERE key = 'speed';

-- Deactivate product-level and obsolete review criteria
UPDATE org.review_criteria
    SET is_active = false
    WHERE context = 'product'
       OR key IN ('delivery_speed', 'product_quality', 'rep_service', 'price_fairness', 'order_accuracy', 'packaging');

COMMIT;