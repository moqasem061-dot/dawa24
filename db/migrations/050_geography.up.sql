-- Migration 050: Geography & Spatial Distance Support
-- Adds latitude, longitude, and regions to cities, coordinates to weekly coverage areas,
-- and the platform.distance_meters function.

BEGIN;

-- 1. Cities Spatial Columns
ALTER TABLE platform_admin.cities
    ADD COLUMN IF NOT EXISTS latitude     NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS longitude    NUMERIC(11,8),
    ADD COLUMN IF NOT EXISTS main_city_id BIGINT REFERENCES platform_admin.cities(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS region       JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::JSONB,
    ADD COLUMN IF NOT EXISTS time_zone    TEXT NOT NULL DEFAULT 'Africa/Cairo',
    ADD COLUMN IF NOT EXISTS is_capital   BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS population   INT,
    ADD COLUMN IF NOT EXISTS area_km2     NUMERIC(12,3);

-- 2. Weekly Coverage Coordinates & City Association
ALTER TABLE workflow.weekly_coverages
    ADD COLUMN IF NOT EXISTS city_id   BIGINT REFERENCES platform_admin.cities(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS latitude  NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS longitude NUMERIC(11,8);

-- 3. Distance in Metres function (Haversine formula, R = 6,371,000 m)
CREATE OR REPLACE FUNCTION platform.distance_meters(
    lat1 NUMERIC, lon1 NUMERIC, lat2 NUMERIC, lon2 NUMERIC
) RETURNS INT LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
    r NUMERIC := 6371000.0;
    phi1 NUMERIC;
    phi2 NUMERIC;
    delta_phi NUMERIC;
    delta_lambda NUMERIC;
    a NUMERIC;
    c NUMERIC;
BEGIN
    IF lat1 IS NULL OR lon1 IS NULL OR lat2 IS NULL OR lon2 IS NULL THEN
        RETURN NULL;
    END IF;

    phi1 := radians(lat1);
    phi2 := radians(lat2);
    delta_phi := radians(lat2 - lat1);
    delta_lambda := radians(lon2 - lon1);

    a := sin(delta_phi / 2.0)^2 + cos(phi1) * cos(phi2) * sin(delta_lambda / 2.0)^2;
    c := 2.0 * atan2(sqrt(a), sqrt(1.0 - a));

    RETURN round(r * c)::INT;
END;
$$;

-- 4. Backfill Coordinates for Egyptian Cities
UPDATE platform_admin.cities SET latitude = 30.04442000, longitude = 31.23571200, is_capital = true, region = '{"ar":"القاهرة الكبرى","en":"Greater Cairo"}'::jsonb WHERE name->>'ar' LIKE '%قاهرة%' OR name->>'en' ILIKE '%cairo%';
UPDATE platform_admin.cities SET latitude = 30.01305600, longitude = 31.20885300, region = '{"ar":"القاهرة الكبرى","en":"Greater Cairo"}'::jsonb WHERE name->>'ar' LIKE '%جيزة%' OR name->>'en' ILIKE '%giza%';
UPDATE platform_admin.cities SET latitude = 31.20009200, longitude = 29.91873900, region = '{"ar":"الساحل الشمالي والإسكندرية","en":"Alexandria & North Coast"}'::jsonb WHERE name->>'ar' LIKE '%إسكندرية%' OR name->>'ar' LIKE '%اسكندرية%' OR name->>'en' ILIKE '%alexandria%';
UPDATE platform_admin.cities SET latitude = 31.04094800, longitude = 31.37847000, region = '{"ar":"الدلتا","en":"Delta"}'::jsonb WHERE name->>'ar' LIKE '%منصورة%' OR name->>'en' ILIKE '%mansoura%';
UPDATE platform_admin.cities SET latitude = 30.78650800, longitude = 31.00037600, region = '{"ar":"الدلتا","en":"Delta"}'::jsonb WHERE name->>'ar' LIKE '%طنطا%' OR name->>'en' ILIKE '%tanta%';
UPDATE platform_admin.cities SET latitude = 30.57650000, longitude = 31.50410000, region = '{"ar":"القناة والدلتا","en":"Delta"}'::jsonb WHERE name->>'ar' LIKE '%زقازيق%' OR name->>'en' ILIKE '%zagazig%';
UPDATE platform_admin.cities SET latitude = 30.60427200, longitude = 32.27225100, region = '{"ar":"مدن القناة","en":"Canal Cities"}'::jsonb WHERE name->>'ar' LIKE '%إسماعيلية%' OR name->>'ar' LIKE '%اسماعيلية%' OR name->>'en' ILIKE '%ismailia%';
UPDATE platform_admin.cities SET latitude = 31.26528900, longitude = 32.30186600, region = '{"ar":"مدن القناة","en":"Canal Cities"}'::jsonb WHERE name->>'ar' LIKE '%بورسعيد%' OR name->>'en' ILIKE '%port said%';
UPDATE platform_admin.cities SET latitude = 29.96683400, longitude = 32.54980600, region = '{"ar":"مدن القناة","en":"Canal Cities"}'::jsonb WHERE name->>'ar' LIKE '%سويس%' OR name->>'en' ILIKE '%suez%';
UPDATE platform_admin.cities SET latitude = 27.18013400, longitude = 31.18368300, region = '{"ar":"صعيد مصر","en":"Upper Egypt"}'::jsonb WHERE name->>'ar' LIKE '%أسيوط%' OR name->>'ar' LIKE '%اسيوط%' OR name->>'en' ILIKE '%assiut%';
UPDATE platform_admin.cities SET latitude = 26.55695000, longitude = 31.69478000, region = '{"ar":"صعيد مصر","en":"Upper Egypt"}'::jsonb WHERE name->>'ar' LIKE '%سوهاج%' OR name->>'en' ILIKE '%sohag%';
UPDATE platform_admin.cities SET latitude = 25.68724300, longitude = 32.63963700, region = '{"ar":"صعيد مصر","en":"Upper Egypt"}'::jsonb WHERE name->>'ar' LIKE '%أقصر%' OR name->>'ar' LIKE '%اقصر%' OR name->>'en' ILIKE '%luxor%';
UPDATE platform_admin.cities SET latitude = 24.08893800, longitude = 32.89982900, region = '{"ar":"صعيد مصر","en":"Upper Egypt"}'::jsonb WHERE name->>'ar' LIKE '%أسوان%' OR name->>'ar' LIKE '%اسوان%' OR name->>'en' ILIKE '%aswan%';

-- Backfill weekly coverages with default coordinates for Cairo/Giza suppliers
UPDATE workflow.weekly_coverages SET latitude = 30.04442000, longitude = 31.23571200, distance_meters = 25000 WHERE latitude IS NULL;

COMMIT;
