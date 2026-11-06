-- 248_order_shipment_wallet_payments.down.sql
BEGIN;

ALTER TABLE commerce.order_shipments
    DROP COLUMN IF EXISTS wallet_refunded_amount,
    DROP COLUMN IF EXISTS wallet_paid_at,
    DROP COLUMN IF EXISTS wallet_paid_amount;

COMMIT;
