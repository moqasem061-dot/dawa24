-- 167_city_coverage_radius.down.sql

BEGIN;

ALTER TABLE platform_admin.cities
    DROP CONSTRAINT IF EXISTS cities_coverage_radius_sane;
ALTER TABLE platform_admin.cities
    DROP COLUMN IF EXISTS coverage_radius_meters;

COMMIT;
