BEGIN;

DROP TABLE IF EXISTS billing.subscriptions CASCADE;
DROP TABLE IF EXISTS billing.plan_features CASCADE;
DROP TABLE IF EXISTS billing.plans CASCADE;
DROP TABLE IF EXISTS billing.payments CASCADE;
DROP TABLE IF EXISTS billing.payment_integrations CASCADE;
DROP TABLE IF EXISTS billing.wallet_transactions CASCADE;
DROP TABLE IF EXISTS billing.wallets CASCADE;

DROP SCHEMA IF EXISTS billing CASCADE;

COMMIT;
