-- Migration 177 Down: drop the structured payment-method fields.
--
-- account_identifier keeps the rendered line, so display survives; only the
-- ability to reopen a method for editing is lost.
BEGIN;
ALTER TABLE billing.user_payment_methods DROP CONSTRAINT IF EXISTS user_payment_methods_provider_check;
ALTER TABLE billing.user_payment_methods DROP COLUMN IF EXISTS details;
COMMIT;
