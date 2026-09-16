ALTER TABLE billing.invoices DROP CONSTRAINT IF EXISTS invoices_status_check;
ALTER TABLE billing.invoices ADD CONSTRAINT invoices_status_check
    CHECK (status IN ('draft', 'issued', 'paid', 'overdue', 'cancelled'));