-- 211_wallet_withdrawal_receipt.up.sql
ALTER TABLE billing.wallet_withdrawals
    ADD COLUMN IF NOT EXISTS transfer_receipt_url TEXT NOT NULL DEFAULT '';
