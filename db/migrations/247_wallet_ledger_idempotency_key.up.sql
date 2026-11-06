-- 247_wallet_ledger_idempotency_key.up.sql
-- Batch 1B (S03): an idempotency key on wallet ledger rows. Order refunds set it
-- so that one cancellation event can credit a wallet at most once. Existing rows
-- keep NULL, and the partial unique index ignores NULL, so the migration cannot
-- fail on existing data. Old code keeps inserting rows without the key.

BEGIN;

ALTER TABLE billing.wallet_transactions
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_wallet_transactions_idempotency_key
    ON billing.wallet_transactions (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

COMMIT;
