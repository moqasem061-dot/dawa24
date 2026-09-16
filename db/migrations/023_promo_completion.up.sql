-- Migration 023: Promo Completion, Ad Plans, Offer Promotions, Location Covers
-- Schema: promo

CREATE TABLE IF NOT EXISTS promo.ad_plans (
    id BIGSERIAL PRIMARY KEY,
    public_id VARCHAR(32) NOT NULL DEFAULT ('pln_' || replace(gen_random_uuid()::text, '-', '')),
    name JSONB NOT NULL,
    position VARCHAR(64) NOT NULL,
    price NUMERIC(15,2) NOT NULL DEFAULT 0.00,
    duration_days INT NOT NULL DEFAULT 30,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS promo.offer_promotions (
    id BIGSERIAL PRIMARY KEY,
    public_id VARCHAR(32) NOT NULL DEFAULT ('opm_' || replace(gen_random_uuid()::text, '-', '')),
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    offer_id BIGINT NOT NULL REFERENCES promo.offers(id) ON DELETE CASCADE,
    plan_id BIGINT REFERENCES promo.ad_plans(id) ON DELETE SET NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    starts_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS promo.offer_location_covers (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    offer_id BIGINT NOT NULL REFERENCES promo.offers(id) ON DELETE CASCADE,
    city_id BIGINT NOT NULL REFERENCES platform_admin.cities(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_offer_location_city UNIQUE (offer_id, city_id)
);

ALTER TABLE promo.offer_promotions ENABLE ROW LEVEL SECURITY;
ALTER TABLE promo.offer_promotions FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_offer_promotions_isolation ON promo.offer_promotions
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id));

ALTER TABLE promo.offer_location_covers ENABLE ROW LEVEL SECURITY;
ALTER TABLE promo.offer_location_covers FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_offer_location_covers_isolation ON promo.offer_location_covers
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id));
