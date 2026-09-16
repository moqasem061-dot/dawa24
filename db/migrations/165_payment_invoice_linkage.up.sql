-- 165_payment_invoice_linkage.up.sql
-- Links billing.payments to billing.invoices to support invoice payments, partial payments, and reconciliation.

BEGIN;

ALTER TABLE billing.payments
    ADD COLUMN IF NOT EXISTS invoice_id BIGINT REFERENCES billing.invoices(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS billing_payments_invoice_id_idx
    ON billing.payments (invoice_id);

CREATE INDEX IF NOT EXISTS billing_payments_org_created_idx
    ON billing.payments (organization_id, created_at DESC);

COMMENT ON COLUMN billing.payments.invoice_id IS
    'معرف الفاتورة المرتبطة بالدفعة لدعم السداد الكلي والجزئي ومطابقة الحسابات';

COMMIT;
