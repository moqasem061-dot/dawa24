-- 167_city_coverage_radius.up.sql
-- Each city gets its own coverage radius, instead of every vendor inventing one.
--
-- Coverage used to be a number the vendor typed: نصف قطر التغطية الافتراضي,
-- defaulting to 5,000 metres, applied identically to every city they selected.
-- Five kilometres from the centre of حدائق الزيتون reaches four unrelated Cairo
-- districts; five kilometres from the centre of أبو سمبل reaches desert. The
-- same number cannot be right for both, and asking a distributor to work out
-- the built-up radius of three hundred and fifty places is not a task anyone
-- completes — so in practice every coverage row on the platform carried the
-- default, and coverage meant nothing.
--
-- The radius belongs to the city, not to the vendor. A vendor selects the cities
-- they deliver to; the platform already knows how large each of those is.
--
-- HOW THE VALUES ARE DERIVED
--
-- Half the distance to the nearest other city, clamped.
--
-- That is a Voronoi half-space: the boundary between two neighbouring places
-- sits midway between their centres, so a radius of half the gap covers a
-- city's own ground and provably cannot reach a neighbour's centre. On the
-- dense Cairo and Giza districts this yields 1.5–3 km, which is about right for
-- a district; on مرسى علم, whose nearest neighbour is 131 km away, the raw half
-- would be 65 km, which is not the size of مرسى علم — it is the size of the
-- empty desert around it.
--
-- Hence the clamps:
--   floor   1,500 m  the smallest useful delivery radius; below it a vendor
--                    covering a district would miss the far side of it.
--   ceiling 12,000 m an ordinary town's built-up extent; beyond this a vendor
--                    should be selecting more cities, not one enormous circle.
--   ceiling 25,000 m for a governorate capital, which genuinely is that large.
--
-- Measured over the 350 cities that carry coordinates: minimum 1,500, mean
-- ~3,700, maximum 12,000; 152 sit at the floor and 20 at the ceiling.
--
-- Any city without coordinates keeps the column default. There is one (القاهرة
-- as a country-level row) and it is not selectable as a delivery target.

BEGIN;

ALTER TABLE platform_admin.cities
    ADD COLUMN IF NOT EXISTS coverage_radius_meters INTEGER NOT NULL DEFAULT 3000;

ALTER TABLE platform_admin.cities
    DROP CONSTRAINT IF EXISTS cities_coverage_radius_sane;
ALTER TABLE platform_admin.cities
    ADD CONSTRAINT cities_coverage_radius_sane
    CHECK (coverage_radius_meters BETWEEN 200 AND 150000);

-- Backfill from the geometry the table already holds.
WITH located AS (
    SELECT id,
           COALESCE(is_capital, FALSE) AS is_capital,
           latitude::float8  AS lat,
           longitude::float8 AS lon
    FROM platform_admin.cities
    WHERE latitude IS NOT NULL AND longitude IS NOT NULL
),
nearest AS (
    SELECT a.id,
           a.is_capital,
           MIN(
               6371000 * 2 * asin(sqrt(
                   power(sin(radians(b.lat - a.lat) / 2), 2)
                   + cos(radians(a.lat)) * cos(radians(b.lat))
                     * power(sin(radians(b.lon - a.lon) / 2), 2)
               ))
           ) AS gap_m
    FROM located a
    JOIN located b ON b.id <> a.id
    GROUP BY a.id, a.is_capital
)
UPDATE platform_admin.cities c
SET coverage_radius_meters = GREATEST(
        1500,
        LEAST(
            n.gap_m / 2,
            CASE WHEN n.is_capital THEN 25000 ELSE 12000 END
        )
    )::int
FROM nearest n
WHERE n.id = c.id;

COMMENT ON COLUMN platform_admin.cities.coverage_radius_meters IS
    'نصف قطر التغطية الجغرافية للمدينة بالمتر — يُطبَّق تلقائياً عند اختيار المدينة في تغطية المورد. محسوب من نصف المسافة إلى أقرب مدينة مجاورة مع حدّ أدنى 1500م وحدّ أقصى 12000م (25000م لعواصم المحافظات)';

COMMIT;
