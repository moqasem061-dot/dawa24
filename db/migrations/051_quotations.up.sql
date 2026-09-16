-- Migration 051: Distance-Based Quotations & Delivery Bands
-- Extends commerce.quote_requests with delivery coordinates and fees,
-- and creates org.delivery_bands for distance-banded delivery pricing.

BEGIN;

-- 1. Extend Quote Requests with Distance & Geolocation Data
ALTER TABLE commerce.quote_requests
    ADD COLUMN IF NOT EXISTS delivery_lat       NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS delivery_lon       NUMERIC(11,8),
    ADD COLUMN IF NOT EXISTS delivery_city_id   BIGINT REFERENCES platform_admin.cities(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS delivery_branch_id BIGINT REFERENCES org.branches(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS requested_for_date DATE,
    ADD COLUMN IF NOT EXISTS distance_meters    INT,
    ADD COLUMN IF NOT EXISTS delivery_fee       NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    ADD COLUMN IF NOT EXISTS expires_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS rejection_reason   TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS quote_requests_delivery_city_idx ON commerce.quote_requests (delivery_city_id);

-- 2. Delivery Bands Table (Distance-tiered delivery fees per organization)
CREATE TABLE IF NOT EXISTS org.delivery_bands (
    id              BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    from_meters     INT NOT NULL DEFAULT 0,
    to_meters       INT NOT NULL DEFAULT 10000,
    fee             NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT delivery_band_range CHECK (to_meters >= from_meters)
);

CREATE INDEX IF NOT EXISTS delivery_bands_org_idx ON org.delivery_bands (organization_id) WHERE is_active = true;

ALTER TABLE org.delivery_bands ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'delivery_bands_tenant_isolation') THEN
        CREATE POLICY delivery_bands_tenant_isolation ON org.delivery_bands
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

COMMIT;
