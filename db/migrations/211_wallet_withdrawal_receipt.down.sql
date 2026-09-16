-- 211_wallet_withdrawal_receipt.down.sql
ALTER TABLE billing.wallet_withdrawals
    DROP COLUMN IF EXISTS transfer_receipt_url;
