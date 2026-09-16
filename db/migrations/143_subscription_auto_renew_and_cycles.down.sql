-- 143_subscription_auto_renew_and_cycles.down.sql

BEGIN;

DROP INDEX IF EXISTS billing.idx_subscriptions_auto_renew;

ALTER TABLE billing.subscriptions
    DROP COLUMN IF EXISTS billing_cycle,
    DROP COLUMN IF EXISTS auto_renew,
    DROP COLUMN IF EXISTS last_renewed_at,
    DROP COLUMN IF EXISTS renewal_attempts;

COMMIT;
