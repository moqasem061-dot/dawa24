-- 223_wallet_deposits_amount_numeric.up.sql
--
-- Fix billing.wallet_deposits.amount column type:
-- Changed from BIGINT to NUMERIC(12,2) to match money.Amount semantics
-- and all other billing/wallet tables (wallet_withdrawals, wallet_transactions, payments).

BEGIN;

ALTER TABLE billing.wallet_deposits
    ALTER COLUMN amount TYPE NUMERIC(12,2) USING (amount::NUMERIC(12,2));

COMMENT ON COLUMN billing.wallet_deposits.amount IS 'مبلغ طلب الشحن بالجنيه — deposit amount in EGP (numeric with 2 decimal places)';

COMMIT;