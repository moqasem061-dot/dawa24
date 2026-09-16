-- 144_subscription_feature_gates.down.sql
BEGIN;

DELETE FROM billing.plan_features
WHERE feature_key IN ('feature_market_discounts', 'feature_compare_tool');

COMMIT;
