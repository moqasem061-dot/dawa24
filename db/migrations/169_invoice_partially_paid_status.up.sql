-- Allow 'partially_paid' in billing.invoices status check constraint so that
-- partial invoice payments can be recorded and tracked accurately.
ALTER TABLE billing.invoices DROP CONSTRAINT IF EXISTS invoices_status_check;
ALTER TABLE billing.invoices ADD CONSTRAINT invoices_status_check
    CHECK (status IN ('draft', 'issued', 'partially_paid', 'paid', 'overdue', 'cancelled'));