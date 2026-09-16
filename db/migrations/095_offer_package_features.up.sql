-- Migration 095: Offer Package Features and Analytics Indexes
-- Schema: promo

CREATE TABLE IF NOT EXISTS promo.offer_package_features (
    id BIGSERIAL PRIMARY KEY,
    public_id VARCHAR(32) NOT NULL DEFAULT ('opf_' || replace(gen_random_uuid()::text, '-', '')),
    package_id BIGINT NOT NULL REFERENCES promo.offer_packages(id) ON DELETE CASCADE,
    feature_name JSONB NOT NULL,
    feature_key VARCHAR(64) NOT NULL,
    feature_value VARCHAR(128) NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_offer_package_features_pkg ON promo.offer_package_features (package_id);

-- Analytics Indexes for Fast Aggregation (Avoid table scans)
CREATE INDEX IF NOT EXISTS idx_offer_views_created_at ON promo.offer_views (created_at);
CREATE INDEX IF NOT EXISTS idx_offer_clicks_created_at ON promo.offer_clicks (created_at);
