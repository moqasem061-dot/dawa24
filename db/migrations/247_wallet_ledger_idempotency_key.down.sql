-- 247_wallet_ledger_idempotency_key.down.sql
-- Removes the order-refund idempotency key. Rows written with a key lose the
-- key only; the credit rows themselves stay in the ledger.

BEGIN;

DROP INDEX IF EXISTS billing.idx_wallet_transactions_idempotency_key;

ALTER TABLE billing.wallet_transactions
    DROP COLUMN IF EXISTS idempotency_key;

COMMIT;
