-- 183_wallet_withdrawals_and_deposit_channels.down.sql

BEGIN;

DROP TABLE IF EXISTS billing.wallet_withdrawals CASCADE;

ALTER TABLE billing.wallet_deposits
    DROP COLUMN IF EXISTS platform_method_id,
    DROP COLUMN IF EXISTS sender_account,
    DROP COLUMN IF EXISTS sender_payment_method_id;

COMMIT;
