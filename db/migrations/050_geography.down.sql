-- Migration 050 Down

BEGIN;

DROP FUNCTION IF EXISTS platform.distance_meters(NUMERIC, NUMERIC, NUMERIC, NUMERIC);

ALTER TABLE workflow.weekly_coverages
    DROP COLUMN IF EXISTS city_id,
    DROP COLUMN IF EXISTS latitude,
    DROP COLUMN IF EXISTS longitude;

ALTER TABLE platform_admin.cities
    DROP COLUMN IF EXISTS latitude,
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS main_city_id,
    DROP COLUMN IF EXISTS region,
    DROP COLUMN IF EXISTS time_zone,
    DROP COLUMN IF EXISTS is_capital,
    DROP COLUMN IF EXISTS population,
    DROP COLUMN IF EXISTS area_km2;

COMMIT;
