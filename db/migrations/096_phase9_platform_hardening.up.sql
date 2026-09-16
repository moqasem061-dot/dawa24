-- Migration 096: Phase 9 Platform Hardening
-- Multi-session & Device Tracking, Session Plan Requests, AI Providers Registry

-- 1. Identity Sessions & Devices
CREATE TABLE IF NOT EXISTS identity.user_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    device_uuid VARCHAR(64) NOT NULL,
    device_name VARCHAR(128) NOT NULL DEFAULT '',
    device_type VARCHAR(32) NOT NULL DEFAULT 'desktop',
    platform VARCHAR(32) NOT NULL DEFAULT '',
    browser VARCHAR(64) NOT NULL DEFAULT '',
    ip_address VARCHAR(45) NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    country VARCHAR(64) NOT NULL DEFAULT '',
    city VARCHAR(64) NOT NULL DEFAULT '',
    logged_in_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    logged_out_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user_active ON identity.user_sessions (user_id, is_active);

CREATE TABLE IF NOT EXISTS identity.user_session_histories (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    session_id BIGINT REFERENCES identity.user_sessions(id) ON DELETE SET NULL,
    device_uuid VARCHAR(64) NOT NULL,
    ip_address VARCHAR(45) NOT NULL DEFAULT '',
    action VARCHAR(64) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity.session_plan_requests (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    requested_plan_id BIGINT REFERENCES identity.session_plans(id) ON DELETE RESTRICT,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    admin_notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 2. Platform Admin AI Providers Registry
CREATE TABLE IF NOT EXISTS platform_admin.ai_providers (
    id BIGSERIAL PRIMARY KEY,
    provider_name VARCHAR(64) NOT NULL UNIQUE,
    model_name VARCHAR(128) NOT NULL,
    capability VARCHAR(64) NOT NULL DEFAULT 'product.match',
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_working BOOLEAN NOT NULL DEFAULT true,
    last_error TEXT,
    context_length INT NOT NULL DEFAULT 4096,
    price_per_1k NUMERIC(10,6) NOT NULL DEFAULT 0.000000,
    base_url VARCHAR(255) NOT NULL DEFAULT '',
    config_key VARCHAR(128) NOT NULL DEFAULT '',
    config_value VARCHAR(255) NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    meta JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
