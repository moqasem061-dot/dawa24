-- 165_payment_invoice_linkage.down.sql
BEGIN;

DROP INDEX IF EXISTS billing.billing_payments_org_created_idx;
DROP INDEX IF EXISTS billing.billing_payments_invoice_id_idx;
ALTER TABLE billing.payments DROP COLUMN IF EXISTS invoice_id;

COMMIT;
