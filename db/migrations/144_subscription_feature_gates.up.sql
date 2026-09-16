-- 144_subscription_feature_gates.up.sql
-- Seed feature keys for market discounts and compare tool across subscription plans.

BEGIN;

-- Ensure billing.plan_features table exists
CREATE TABLE IF NOT EXISTS billing.plan_features (
    plan_id BIGINT NOT NULL REFERENCES billing.plans(id) ON DELETE CASCADE,
    feature_key TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT 'true',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (plan_id, feature_key)
);

-- Seed feature gates for Pro and Enterprise tiers
INSERT INTO billing.plan_features (plan_id, feature_key, value)
SELECT p.id, f.key, 'true'
FROM billing.plans p
CROSS JOIN (
    VALUES 
        ('feature_market_discounts'),
        ('feature_compare_tool')
) AS f(key)
WHERE p.slug IN ('pro', 'enterprise')
ON CONFLICT (plan_id, feature_key) DO UPDATE SET value = 'true';

-- Ensure basic tier has them explicitly set to false unless updated by admin
INSERT INTO billing.plan_features (plan_id, feature_key, value)
SELECT p.id, f.key, 'false'
FROM billing.plans p
CROSS JOIN (
    VALUES 
        ('feature_market_discounts'),
        ('feature_compare_tool')
) AS f(key)
WHERE p.slug = 'basic'
ON CONFLICT (plan_id, feature_key) DO NOTHING;

COMMIT;
