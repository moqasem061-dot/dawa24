-- 235_savings_match_decision.down.sql

BEGIN;

DROP INDEX IF EXISTS catalog.saving_products_match_state;

ALTER TABLE catalog.saving_products
    DROP COLUMN IF EXISTS baseline_decision,
    DROP COLUMN IF EXISTS match_decision,
    DROP COLUMN IF EXISTS match_run_id,
    DROP COLUMN IF EXISTS ai_state,
    DROP COLUMN IF EXISTS human_locked,
    DROP COLUMN IF EXISTS match_revision,
    DROP COLUMN IF EXISTS match_granularity,
    DROP COLUMN IF EXISTS match_basis,
    DROP COLUMN IF EXISTS match_state;

DROP INDEX IF EXISTS platform.import_run_rows_match_state;

ALTER TABLE platform.import_run_rows
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
