-- 065_merge_special_offers (up)
--
-- Rebuild V2 §2.2 — promo.special_offers/_products/_locations duplicate the
-- offers family. The special_* rows migrate into promo.offers /
-- promo.offer_products / promo.offer_location_covers, keeping the richer
-- column set (image, total_price, is_draft, and the per-location address,
-- coordinates, radius, day and time window). The three tables are dropped.
--
-- Legacy rows were never seeded, so the column mappings below are the
-- documented contract (see docs/modules/promo.md):
--   * status: draft -> is_draft, active -> is_active, else inactive
--   * discount: percentage/fixed -> discount_type + discount_value
--   * dates: DATE -> starts_at/expires_at (TIMESTAMPTZ)
--   * day_of_week: special_offers used 1..7, offers use 0..6 (0 = Sunday)
--   * city-less special locations are dropped (city_id is NOT NULL)

BEGIN;

-- 1. Enrich promo.offers with the special_offer extras that have no home yet.
ALTER TABLE promo.offers
    ADD COLUMN IF NOT EXISTS source      TEXT NOT NULL DEFAULT 'standard'
        CHECK (source IN ('standard','special')),
    ADD COLUMN IF NOT EXISTS is_draft    BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS image       TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS total_price NUMERIC(10,2) NOT NULL DEFAULT 0.00;

-- min_order_amount (062) is canonical; min_order_value never survived a read
-- after this migration (grep internal/ for min_order_value before merging).
ALTER TABLE promo.offers DROP COLUMN IF EXISTS min_order_value;

-- 2. Migrate the special offer masters, preserving ids and public ids so
--    order/line and location links keep their meaning.
INSERT INTO promo.offers (
    id, public_id, organization_id, branch_id, title, description,
    discount_type, discount_value, min_order_amount, total_price,
    starts_at, expires_at, is_active, is_draft, admin_status, image, source
)
SELECT so.id, so.public_id, so.organization_id, so.branch_id, so.title, so.description,
       CASE WHEN so.discount_amount > 0 THEN 'fixed' ELSE 'percentage' END,
       CASE WHEN so.discount_amount > 0 THEN so.discount_amount
            ELSE COALESCE(so.discount_percentage, 0) END,
       COALESCE(so.min_order_amount, 0), COALESCE(so.total_price, 0),
       COALESCE(so.start_date::timestamptz, now()),
       COALESCE(so.end_date::timestamptz, now() + interval '30 days'),
       so.status = 'active', so.status = 'draft',
       so.admin_status, COALESCE(so.image, ''), 'special'
FROM promo.special_offers so
ON CONFLICT (id) DO NOTHING;

-- Keep the identity sequence ahead of the preserved ids.
SELECT setval(pg_get_serial_sequence('promo.offers', 'id'),
              (SELECT COALESCE(MAX(id), 0) + 1 FROM promo.offers), false);

-- 3. Migrate the special offer products onto offer_products. Rows whose
--    variant lost its product are dropped (product_id is NOT NULL there).
INSERT INTO promo.offer_products (
    offer_id, product_id, variant_id, custom_price,
    custom_discount_percentage, custom_discount_amount, custom_qty
)
SELECT sop.offer_id, pv.product_id, sop.variant_id, sop.custom_price,
       sop.discount_percentage, sop.discount_amount, sop.quantity
FROM promo.special_offer_products sop
JOIN catalog.product_variants pv ON pv.id = sop.variant_id
WHERE pv.product_id IS NOT NULL;

-- 4. Enrich offer_location_covers with the special location fields, then
--    migrate the rows (day_of_week shifts from 1..7 to 0..6).
ALTER TABLE promo.offer_location_covers
    ADD COLUMN IF NOT EXISTS address_ar   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS address_en   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS latitude     NUMERIC(10,8) NOT NULL DEFAULT 30.0444,
    ADD COLUMN IF NOT EXISTS longitude    NUMERIC(11,8) NOT NULL DEFAULT 31.2357,
    ADD COLUMN IF NOT EXISTS radius_meters INT NOT NULL DEFAULT 500,
    ADD COLUMN IF NOT EXISTS day_of_week  INT CHECK (day_of_week BETWEEN 0 AND 6),
    ADD COLUMN IF NOT EXISTS time_from    TIME,
    ADD COLUMN IF NOT EXISTS time_to      TIME,
    ADD COLUMN IF NOT EXISTS status       TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active','inactive')),
    ADD COLUMN IF NOT EXISTS admin_status TEXT NOT NULL DEFAULT 'approved'
        CHECK (admin_status IN ('pending','approved','rejected'));

INSERT INTO promo.offer_location_covers (
    organization_id, offer_id, city_id, address_ar, address_en,
    latitude, longitude, radius_meters, day_of_week, time_from, time_to,
    status, admin_status
)
SELECT so.organization_id, sl.offer_id, sl.city_id, sl.address_ar, sl.address_en,
       sl.latitude, sl.longitude, sl.radius, sl.day_of_week - 1, sl.time_from, sl.time_to,
       sl.status, sl.admin_status
FROM promo.special_offer_locations sl
JOIN promo.special_offers so ON so.id = sl.offer_id
WHERE sl.city_id IS NOT NULL
ON CONFLICT (offer_id, city_id) DO NOTHING;

CREATE INDEX IF NOT EXISTS offer_location_covers_day_idx ON promo.offer_location_covers (offer_id, day_of_week);

-- 5. Drop the duplicate family.
DROP TABLE promo.special_offer_locations;
DROP TABLE promo.special_offer_products;
DROP TABLE promo.special_offers;

COMMENT ON COLUMN promo.offers.source IS 'أصل العرض — standard: عروض الموردين، special: عروض المؤسسة الخاصة (065)';
COMMENT ON COLUMN promo.offers.is_draft IS 'مسودة لم تُنشر بعد — draft from the legacy special_offers.status (065)';
COMMENT ON COLUMN promo.offers.image IS 'صورة العرض';
COMMENT ON COLUMN promo.offers.total_price IS 'السعر الإجمالي الكلي للعرض — total_price (065)';
COMMENT ON COLUMN promo.offer_location_covers.address_ar IS 'عنوان التغطية بالعربية (065)';
COMMENT ON COLUMN promo.offer_location_covers.latitude IS 'خط العرض لنقطة التغطية (065)';
COMMENT ON COLUMN promo.offer_location_covers.longitude IS 'خط الطول لنقطة التغطية (065)';
COMMENT ON COLUMN promo.offer_location_covers.radius_meters IS 'نصف قطر التغطية بالأمتار (065)';
COMMENT ON COLUMN promo.offer_location_covers.day_of_week IS 'يوم التغطية — 0 = الأحد (065)';
COMMENT ON COLUMN promo.offer_location_covers.time_from IS 'بداية فترة التغطية (065)';
COMMENT ON COLUMN promo.offer_location_covers.time_to IS 'نهاية فترة التغطية (065)';
COMMENT ON COLUMN promo.offer_location_covers.status IS 'حالة التغطية — active | inactive (065)';
COMMENT ON COLUMN promo.offer_location_covers.admin_status IS 'اعتماد المنصة للتغطية — pending | approved | rejected (065)';

COMMIT;