-- 090_purchase_priority_engine_columns.down.sql

BEGIN;

DROP POLICY IF EXISTS purchase_priority_tenant_isolation ON workflow.purchase_priority_engines;

ALTER TABLE workflow.purchase_priority_engines
    DROP COLUMN IF EXISTS priorities,
    DROP COLUMN IF EXISTS processing_parameters,
    DROP COLUMN IF EXISTS matched_products,
    DROP COLUMN IF EXISTS ranking_results,
    DROP COLUMN IF EXISTS notes,
    DROP COLUMN IF EXISTS meta,
    DROP COLUMN IF EXISTS processed_by;

COMMIT;
