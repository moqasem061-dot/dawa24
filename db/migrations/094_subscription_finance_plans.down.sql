-- 094_subscription_finance_plans.down.sql

DROP TABLE IF EXISTS billing.user_plan_histories CASCADE;
DROP TABLE IF EXISTS billing.subscription_users CASCADE;
DROP TABLE IF EXISTS billing.subscription_histories CASCADE;
DROP TABLE IF EXISTS billing.plan_types CASCADE;
