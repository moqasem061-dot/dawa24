-- 143_subscription_auto_renew_and_cycles.up.sql
-- Adds billing cycle duration, auto-renewal flag, renewal timestamp, and retry tracking to billing.subscriptions.

BEGIN;

ALTER TABLE billing.subscriptions
    ADD COLUMN IF NOT EXISTS billing_cycle TEXT NOT NULL DEFAULT 'monthly',
    ADD COLUMN IF NOT EXISTS auto_renew BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS last_renewed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS renewal_attempts INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_subscriptions_auto_renew 
ON billing.subscriptions (auto_renew, status, expires_at) 
WHERE auto_renew = true AND status = 'active';

COMMIT;
