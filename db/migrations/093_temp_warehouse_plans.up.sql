-- 093_temp_warehouse_plans.up.sql
-- Migration 093: Temporary Warehouses Hierarchy and Subscription Plans

BEGIN;

CREATE TABLE IF NOT EXISTS inventory.father_user_temparte_warehouses (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    organization_id BIGINT REFERENCES org.organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_father_temp_user ON inventory.father_user_temparte_warehouses(user_id);
CREATE INDEX IF NOT EXISTS idx_father_temp_org ON inventory.father_user_temparte_warehouses(organization_id);

CREATE TABLE IF NOT EXISTS inventory.plan_temparte_warehouses (
    id              BIGSERIAL PRIMARY KEY,
    name            JSONB NOT NULL,
    description     TEXT,
    price           NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    duration_days   INT NOT NULL DEFAULT 30,
    max_warehouses  INT NOT NULL DEFAULT 5,
    max_rows        INT NOT NULL DEFAULT 10000,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS inventory.user_plan_temparte_warehouses (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    organization_id BIGINT REFERENCES org.organizations(id) ON DELETE CASCADE,
    plan_id         BIGINT NOT NULL REFERENCES inventory.plan_temparte_warehouses(id) ON DELETE CASCADE,
    starts_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'expired', 'cancelled', 'pending')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_plan_temp_user ON inventory.user_plan_temparte_warehouses(user_id);

ALTER TABLE inventory.temp_warehouses
    ADD COLUMN IF NOT EXISTS father_id BIGINT REFERENCES inventory.father_user_temparte_warehouses(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS file_path TEXT,
    ADD COLUMN IF NOT EXISTS row_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

COMMIT;
