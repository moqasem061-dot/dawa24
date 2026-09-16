-- 202_wallet_refunds.down.sql

BEGIN;

DROP INDEX IF EXISTS billing.idx_wallet_deposits_refund_tx;
DROP INDEX IF EXISTS billing.idx_wallet_transactions_reverses_tx;

ALTER TABLE billing.wallet_deposits
    DROP COLUMN IF EXISTS refund_transaction_id,
    DROP COLUMN IF EXISTS refunded_at,
    DROP COLUMN IF EXISTS refunded_by;

ALTER TABLE billing.wallet_deposits
    DROP CONSTRAINT IF EXISTS wallet_deposits_status_check;

ALTER TABLE billing.wallet_deposits
    ADD CONSTRAINT wallet_deposits_status_check
    CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled'));

ALTER TABLE billing.wallet_transactions
    DROP COLUMN IF EXISTS refunded_at,
    DROP COLUMN IF EXISTS refunded_by,
    DROP COLUMN IF EXISTS reverses_transaction_id;

COMMIT;
