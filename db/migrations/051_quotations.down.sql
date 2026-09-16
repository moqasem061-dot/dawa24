-- Migration 051 Down

BEGIN;

DROP TABLE IF EXISTS org.delivery_bands;

ALTER TABLE commerce.quote_requests
    DROP COLUMN IF EXISTS delivery_lat,
    DROP COLUMN IF EXISTS delivery_lon,
    DROP COLUMN IF EXISTS delivery_city_id,
    DROP COLUMN IF EXISTS delivery_branch_id,
    DROP COLUMN IF EXISTS requested_for_date,
    DROP COLUMN IF EXISTS distance_meters,
    DROP COLUMN IF EXISTS delivery_fee,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS rejection_reason;

COMMIT;
