-- Migration 053 Down

BEGIN;

DROP TABLE IF EXISTS org.review_ratings;
DROP TABLE IF EXISTS org.review_criteria;

ALTER TABLE org.organization_reviews
    DROP COLUMN IF EXISTS order_id,
    DROP COLUMN IF EXISTS product_id,
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS response,
    DROP COLUMN IF EXISTS response_at,
    DROP COLUMN IF EXISTS responded_by,
    DROP COLUMN IF EXISTS is_verified,
    DROP COLUMN IF EXISTS is_public,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS helpful_count,
    DROP COLUMN IF EXISTS context,
    DROP COLUMN IF EXISTS deleted_at;

COMMIT;
