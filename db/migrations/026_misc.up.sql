-- Migration 026: User Favorites, Admin Notifications, Supplier Tracking
-- Schemas: identity, notifications, inventory

CREATE TABLE IF NOT EXISTS identity.user_favorites (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES catalog.products(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_user_favorite_product UNIQUE (user_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_user_favorites_user ON identity.user_favorites(user_id);

CREATE TABLE IF NOT EXISTS notifications.admin_notifications (
    id BIGSERIAL PRIMARY KEY,
    event_type VARCHAR(64) NOT NULL,
    title JSONB NOT NULL,
    body JSONB NOT NULL,
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_read BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS inventory.supplier_trackings (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    supplier_org_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    reliability_score NUMERIC(5,2) NOT NULL DEFAULT 100.00,
    fulfillment_rate NUMERIC(5,2) NOT NULL DEFAULT 100.00,
    notes TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_supplier_tracking_orgs UNIQUE (organization_id, supplier_org_id)
);

ALTER TABLE inventory.supplier_trackings ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory.supplier_trackings FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_supplier_trackings_isolation ON inventory.supplier_trackings
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id));
