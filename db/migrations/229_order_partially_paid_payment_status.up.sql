-- Migration 229: Expand commerce.orders payment_status check constraint to support 'partially_paid'.
-- This prevents database transaction rollbacks when recording partial payments on invoices or orders.

ALTER TABLE commerce.orders DROP CONSTRAINT IF EXISTS orders_payment_status_check;
ALTER TABLE commerce.orders ADD CONSTRAINT orders_payment_status_check
    CHECK (payment_status IN ('unpaid', 'authorized', 'partially_paid', 'paid', 'partially_refunded', 'refunded', 'failed'));
