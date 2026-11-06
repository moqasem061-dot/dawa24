-- 248_order_shipment_wallet_payments.up.sql
-- Batch 2 (S02): a wallet-paid order is charged one supplier shipment at a time,
-- when that supplier confirms it. These columns record, per shipment, what was
-- charged and what has been refunded. They are NULL or zero for every existing
-- row, so old orders keep their order-level payment history and old code keeps
-- working. wallet_paid_at is set once, and only in the transaction that debits
-- the wallet.

BEGIN;

ALTER TABLE commerce.order_shipments
    ADD COLUMN IF NOT EXISTS wallet_paid_amount      NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    ADD COLUMN IF NOT EXISTS wallet_paid_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS wallet_refunded_amount  NUMERIC(12,2) NOT NULL DEFAULT 0.00;

COMMIT;
