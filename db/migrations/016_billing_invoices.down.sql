BEGIN;

DROP TABLE IF EXISTS billing.user_payment_methods CASCADE;
DROP TABLE IF EXISTS billing.invoice_lines CASCADE;
DROP TABLE IF EXISTS billing.invoices CASCADE;

COMMIT;
