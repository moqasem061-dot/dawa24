-- Migration 229 Down
ALTER TABLE commerce.orders DROP CONSTRAINT IF EXISTS orders_payment_status_check;
ALTER TABLE commerce.orders ADD CONSTRAINT orders_payment_status_check
    CHECK (payment_status IN ('unpaid', 'authorized', 'paid', 'partially_refunded', 'refunded', 'failed'));
