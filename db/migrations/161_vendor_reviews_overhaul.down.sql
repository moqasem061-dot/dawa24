BEGIN;

DROP INDEX IF EXISTS org.reviews_one_per_vendor_order;
DROP INDEX IF EXISTS org.idx_org_organization_reviews_shipment_id;
DROP INDEX IF EXISTS org.idx_org_organization_reviews_reviewer_org;

ALTER TABLE org.organization_reviews
    DROP COLUMN IF EXISTS shipment_id,
    DROP COLUMN IF EXISTS reviewer_org_id;

CREATE UNIQUE INDEX IF NOT EXISTS reviews_one_per_order
    ON org.organization_reviews (user_id, order_id)
    WHERE order_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS org_reviews_user_unique
    ON org.organization_reviews (organization_id, user_id);

COMMIT;