-- 223_wallet_deposits_amount_numeric.down.sql
-- Revert billing.wallet_deposits.amount back to BIGINT

BEGIN;

ALTER TABLE billing.wallet_deposits
    ALTER COLUMN amount TYPE BIGINT USING (amount::BIGINT);

COMMIT;