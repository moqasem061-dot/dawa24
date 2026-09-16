-- 094_subscription_finance_plans.up.sql
-- Plan types, subscription histories, subscription users, and user plan histories.

CREATE TABLE IF NOT EXISTS billing.plan_types (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(64) NOT NULL UNIQUE,
    description TEXT,
    sort_order INT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS billing.subscription_histories (
    id BIGSERIAL PRIMARY KEY,
    subscription_id BIGINT,
    organization_id BIGINT NOT NULL,
    user_id BIGINT,
    plan_id BIGINT NOT NULL,
    action VARCHAR(64) NOT NULL, -- created, renewed, upgraded, downgraded, cancelled, expired
    amount_minor BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'EGP',
    details TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_histories_org ON billing.subscription_histories (organization_id);
CREATE INDEX IF NOT EXISTS idx_subscription_histories_plan ON billing.subscription_histories (plan_id);

CREATE TABLE IF NOT EXISTS billing.subscription_users (
    id BIGSERIAL PRIMARY KEY,
    subscription_id BIGINT,
    organization_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    role VARCHAR(64) NOT NULL DEFAULT 'member',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_users_user ON billing.subscription_users (user_id);
CREATE INDEX IF NOT EXISTS idx_subscription_users_org ON billing.subscription_users (organization_id);

CREATE TABLE IF NOT EXISTS billing.user_plan_histories (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    organization_id BIGINT,
    plan_id BIGINT NOT NULL,
    start_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    end_date TIMESTAMPTZ,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_plan_histories_user ON billing.user_plan_histories (user_id);
