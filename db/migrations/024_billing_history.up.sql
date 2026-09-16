-- Migration 024: Payment Histories & Audit Trail
-- Schema: billing

CREATE TABLE IF NOT EXISTS billing.payment_histories (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    payment_id BIGINT REFERENCES billing.payments(id) ON DELETE SET NULL,
    invoice_id BIGINT REFERENCES billing.invoices(id) ON DELETE SET NULL,
    action VARCHAR(64) NOT NULL,
    amount NUMERIC(15,2) NOT NULL DEFAULT 0.00,
    status VARCHAR(32) NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE billing.payment_histories ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing.payment_histories FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_payment_histories_isolation ON billing.payment_histories
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id));
