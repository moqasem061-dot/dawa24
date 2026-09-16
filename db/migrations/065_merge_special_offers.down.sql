-- 065_merge_special_offers (down)
--
-- Recreates the special_offers family from the offers rows that carry
-- source = 'special', restoring the 1..7 day_of_week numbering.

BEGIN;

CREATE TABLE promo.special_offers (
    id                  BIGSERIAL PRIMARY KEY,
    public_id           UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    branch_id           BIGINT REFERENCES org.branches(id) ON DELETE SET NULL,
    title               JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
    description         JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
    discount_percentage NUMERIC(5,2) DEFAULT 0.00,
    discount_amount     NUMERIC(12,2) DEFAULT 0.00,
    min_order_amount    NUMERIC(12,2) DEFAULT 0.00,
    total_price         NUMERIC(12,2) DEFAULT 0.00,
    start_date          DATE,
    end_date            DATE,
    status              TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive','expired','draft')),
    admin_status        TEXT NOT NULL DEFAULT 'approved' CHECK (admin_status IN ('pending','approved','rejected')),
    image               TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_special_offers_org    ON promo.special_offers (organization_id);
CREATE INDEX idx_special_offers_branch ON promo.special_offers (branch_id);
CREATE INDEX idx_special_offers_status ON promo.special_offers (status, admin_status);

CREATE TABLE promo.special_offer_products (
    id                  BIGSERIAL PRIMARY KEY,
    offer_id            BIGINT NOT NULL REFERENCES promo.special_offers(id) ON DELETE CASCADE,
    variant_id          BIGINT NOT NULL REFERENCES catalog.product_variants(id) ON DELETE CASCADE,
    custom_price        NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    discount_percentage NUMERIC(5,2) NOT NULL DEFAULT 0.00,
    discount_amount     NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    quantity            INT NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_special_offer_prods_offer ON promo.special_offer_products (offer_id);
CREATE INDEX idx_special_offer_prods_var   ON promo.special_offer_products (variant_id);

CREATE TABLE promo.special_offer_locations (
    id                  BIGSERIAL PRIMARY KEY,
    offer_id            BIGINT NOT NULL REFERENCES promo.special_offers(id) ON DELETE CASCADE,
    city_id             BIGINT REFERENCES platform_admin.cities(id) ON DELETE SET NULL,
    address_ar          TEXT NOT NULL DEFAULT '',
    address_en          TEXT NOT NULL DEFAULT '',
    latitude            NUMERIC(10,8) NOT NULL DEFAULT 30.0444,
    longitude           NUMERIC(11,8) NOT NULL DEFAULT 31.2357,
    radius              INT NOT NULL DEFAULT 500,
    day_of_week         INT NOT NULL DEFAULT 1 CHECK (day_of_week BETWEEN 1 AND 7),
    time_from           TIME,
    time_to             TIME,
    status              TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
    admin_status        TEXT NOT NULL DEFAULT 'approved' CHECK (admin_status IN ('pending','approved','rejected')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_special_offer_locs_offer ON promo.special_offer_locations (offer_id);
CREATE INDEX idx_special_offer_locs_city  ON promo.special_offer_locations (city_id);

INSERT INTO promo.special_offers (
    id, public_id, organization_id, branch_id, title, description,
    discount_percentage, discount_amount, min_order_amount, total_price,
    start_date, end_date, status, admin_status, image, created_at, updated_at
)
SELECT id, public_id, organization_id, branch_id, title, description,
       CASE WHEN discount_type = 'percentage' THEN discount_value ELSE 0 END,
       CASE WHEN discount_type = 'fixed'      THEN discount_value ELSE 0 END,
       min_order_amount, total_price,
       starts_at::date, expires_at::date,
       CASE WHEN is_draft   THEN 'draft'
            WHEN is_active  THEN 'active'
            ELSE 'inactive' END,
       admin_status, image, created_at, updated_at
FROM promo.offers
WHERE source = 'special';

SELECT setval(pg_get_serial_sequence('promo.special_offers', 'id'),
              (SELECT COALESCE(MAX(id), 0) + 1 FROM promo.special_offers), false);

INSERT INTO promo.special_offer_products (offer_id, variant_id, custom_price, discount_percentage, discount_amount, quantity)
SELECT op.offer_id, op.variant_id, op.custom_price, op.custom_discount_percentage, op.custom_discount_amount, op.custom_qty
FROM promo.offer_products op
JOIN promo.offers o ON o.id = op.offer_id
WHERE o.source = 'special'
  AND op.variant_id IS NOT NULL;

INSERT INTO promo.special_offer_locations (
    offer_id, city_id, address_ar, address_en, latitude, longitude,
    radius, day_of_week, time_from, time_to, status, admin_status
)
SELECT olc.offer_id, olc.city_id, olc.address_ar, olc.address_en, olc.latitude, olc.longitude,
       olc.radius_meters, olc.day_of_week + 1, olc.time_from, olc.time_to,
       olc.status, olc.admin_status
FROM promo.offer_location_covers olc
JOIN promo.offers o ON o.id = olc.offer_id
WHERE o.source = 'special';

-- 6. Strip the merged extras back off the offers family.
ALTER TABLE promo.offer_location_covers
    DROP COLUMN IF EXISTS admin_status,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS time_to,
    DROP COLUMN IF EXISTS time_from,
    DROP COLUMN IF EXISTS day_of_week,
    DROP COLUMN IF EXISTS radius_meters,
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS latitude,
    DROP COLUMN IF EXISTS address_en,
    DROP COLUMN IF EXISTS address_ar;

DROP INDEX IF EXISTS promo.offer_location_covers_day_idx;

ALTER TABLE promo.offers
    ADD COLUMN IF NOT EXISTS min_order_value NUMERIC(10,2) NOT NULL DEFAULT 0.00,
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS is_draft,
    DROP COLUMN IF EXISTS image,
    DROP COLUMN IF EXISTS total_price;

COMMIT;