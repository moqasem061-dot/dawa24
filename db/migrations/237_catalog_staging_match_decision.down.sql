-- 237_catalog_staging_match_decision.down.sql
--
-- Reverts product matching decision columns and proposal on catalog.import_staging_rows.

BEGIN;

DROP INDEX IF EXISTS catalog.catalog_import_staging_rows_proposal;
DROP INDEX IF EXISTS catalog.catalog_import_staging_rows_match_state;

ALTER TABLE catalog.import_staging_rows
    DROP COLUMN IF EXISTS proposal,
    DROP COLUMN IF EXISTS baseline_decision,
    DROP COLUMN IF EXISTS match_decision,
    DROP COLUMN IF EXISTS match_run_id,
    DROP COLUMN IF EXISTS ai_state,
    DROP COLUMN IF EXISTS human_locked,
    DROP COLUMN IF EXISTS match_revision,
    DROP COLUMN IF EXISTS match_granularity,
    DROP COLUMN IF EXISTS match_basis,
    DROP COLUMN IF EXISTS match_state;

COMMIT;
