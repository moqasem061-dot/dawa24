-- 202_wallet_refunds.up.sql
-- Adds compensating refund linkage to billing.wallet_transactions
-- and refund audit tracking to billing.wallet_deposits.

BEGIN;

-- 1. Add refund reference columns to billing.wallet_transactions
ALTER TABLE billing.wallet_transactions
    ADD COLUMN IF NOT EXISTS reverses_transaction_id BIGINT REFERENCES billing.wallet_transactions(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS refunded_by BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS refunded_at TIMESTAMPTZ;

-- Unique partial index to ensure one transaction can only be refunded once (idempotent)
CREATE UNIQUE INDEX IF NOT EXISTS idx_wallet_transactions_reverses_tx
    ON billing.wallet_transactions (reverses_transaction_id)
    WHERE reverses_transaction_id IS NOT NULL;

-- 2. Add refund tracking columns to billing.wallet_deposits and allow 'refunded' status
ALTER TABLE billing.wallet_deposits
    DROP CONSTRAINT IF EXISTS wallet_deposits_status_check;

ALTER TABLE billing.wallet_deposits
    ADD CONSTRAINT wallet_deposits_status_check
    CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled', 'refunded'));

ALTER TABLE billing.wallet_deposits
    ADD COLUMN IF NOT EXISTS refunded_by BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS refunded_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS refund_transaction_id BIGINT REFERENCES billing.wallet_transactions(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_wallet_deposits_refund_tx
    ON billing.wallet_deposits (refund_transaction_id)
    WHERE refund_transaction_id IS NOT NULL;

COMMIT;
