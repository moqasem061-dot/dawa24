-- Migration 172: Special Offers tables ensure (repair for 058 misses)
--
-- Rebuilds the promo.special_offers family when migration 058 never landed on
-- a database (cart/checkout reads promo.special_offers on every request, so a
-- database without it fails the whole cart with
-- ERROR: relation "promo.special_offers" does not exist).
--
-- Fully idempotent: every statement is IF NOT EXISTS / conditional, so on a
-- database where 058 applied this migration changes nothing. Never edits 058
-- itself — the runner checksums applied migrations and refuses edited ones.

BEGIN;

-- 1. Branch Institutional Works Table
CREATE TABLE IF NOT EXISTS org.branch_institutional_works (
    id              BIGSERIAL PRIMARY KEY,
    branch_id       BIGINT NOT NULL REFERENCES org.branches(id) ON DELETE CASCADE,
    work_category   TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_branch_institutional_work UNIQUE (branch_id, work_category)
);

CREATE INDEX IF NOT EXISTS idx_branch_institutional_works_branch ON org.branch_institutional_works (branch_id);
CREATE INDEX IF NOT EXISTS idx_branch_institutional_works_cat    ON org.branch_institutional_works (work_category);

ALTER TABLE org.branch_institutional_works ENABLE ROW LEVEL SECURITY;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'branch_inst_works_tenant_isolation') THEN
        CREATE POLICY branch_inst_works_tenant_isolation ON org.branch_institutional_works
            USING (true) WITH CHECK (true);
    END IF;
END $$;

-- 2. Special Offers Table
CREATE TABLE IF NOT EXISTS promo.special_offers (
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

CREATE INDEX IF NOT EXISTS idx_special_offers_org    ON promo.special_offers (organization_id);
CREATE INDEX IF NOT EXISTS idx_special_offers_branch ON promo.special_offers (branch_id);
CREATE INDEX IF NOT EXISTS idx_special_offers_status ON promo.special_offers (status, admin_status);

-- 3. Special Offer Products
CREATE TABLE IF NOT EXISTS promo.special_offer_products (
    id                  BIGSERIAL PRIMARY KEY,
    offer_id            BIGINT NOT NULL REFERENCES promo.special_offers(id) ON DELETE CASCADE,
    variant_id          BIGINT NOT NULL REFERENCES catalog.product_variants(id) ON DELETE CASCADE,
    custom_price        NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    discount_percentage NUMERIC(5,2) NOT NULL DEFAULT 0.00,
    discount_amount     NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    quantity            INT NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_special_offer_prods_offer ON promo.special_offer_products (offer_id);
CREATE INDEX IF NOT EXISTS idx_special_offer_prods_var   ON promo.special_offer_products (variant_id);

-- 4. Special Offer Location Coverage
CREATE TABLE IF NOT EXISTS promo.special_offer_locations (
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

CREATE INDEX IF NOT EXISTS idx_special_offer_locs_offer ON promo.special_offer_locations (offer_id);
CREATE INDEX IF NOT EXISTS idx_special_offer_locs_city  ON promo.special_offer_locations (city_id);

ALTER TABLE promo.special_offers ENABLE ROW LEVEL SECURITY;
ALTER TABLE promo.special_offer_products ENABLE ROW LEVEL SECURITY;
ALTER TABLE promo.special_offer_locations ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'special_offers_tenant_isolation') THEN
        CREATE POLICY special_offers_tenant_isolation ON promo.special_offers USING (true) WITH CHECK (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'special_offer_prods_tenant_isolation') THEN
        CREATE POLICY special_offer_prods_tenant_isolation ON promo.special_offer_products USING (true) WITH CHECK (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'special_offer_locs_tenant_isolation') THEN
        CREATE POLICY special_offer_locs_tenant_isolation ON promo.special_offer_locations USING (true) WITH CHECK (true);
    END IF;
END $$;

COMMIT;
