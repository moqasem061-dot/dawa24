-- 238_compare_match_decision.down.sql

BEGIN;

DROP INDEX IF EXISTS compare.compare_file_rows_group_basis;
DROP INDEX IF EXISTS compare.compare_file_rows_match_state;

ALTER TABLE compare.file_rows
    DROP COLUMN IF EXISTS group_basis,
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
