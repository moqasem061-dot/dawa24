-- 087_compare_plans.down.sql
BEGIN;

DROP TABLE IF EXISTS compare.user_sessions CASCADE;
DROP TABLE IF EXISTS compare.subscription_users CASCADE;
DROP TABLE IF EXISTS compare.subscriptions CASCADE;
DROP TABLE IF EXISTS compare.plan_requests CASCADE;
DROP TABLE IF EXISTS compare.plan_features CASCADE;
DROP TABLE IF EXISTS compare.plans CASCADE;

DROP SCHEMA IF EXISTS compare CASCADE;

COMMIT;
