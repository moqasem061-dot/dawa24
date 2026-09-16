-- 090_purchase_priority_engine_columns.up.sql
--
-- Reconcile workflow.purchase_priority_engines columns with Laravel parity (Plan V5 Phase 3 Task 3.2).

BEGIN;

ALTER TABLE workflow.purchase_priority_engines
    ADD COLUMN IF NOT EXISTS priorities            JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS processing_parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS matched_products      JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS ranking_results       JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS notes                 TEXT,
    ADD COLUMN IF NOT EXISTS meta                  JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS processed_by          BIGINT REFERENCES identity.users(id) ON DELETE SET NULL;

ALTER TABLE workflow.purchase_priority_engines ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow.purchase_priority_engines FORCE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE schemaname = 'workflow'
          AND tablename = 'purchase_priority_engines'
          AND policyname = 'purchase_priority_tenant_isolation'
    ) THEN
        CREATE POLICY purchase_priority_tenant_isolation ON workflow.purchase_priority_engines
            USING (platform.tenant_visible(organization_id) OR user_id = (NULLIF(current_setting('request.jwt.claim.sub', true), ''))::bigint)
            WITH CHECK (platform.tenant_visible(organization_id) OR user_id = (NULLIF(current_setting('request.jwt.claim.sub', true), ''))::bigint);
    END IF;
END $$;

COMMIT;
