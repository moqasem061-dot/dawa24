-- 236_smartorder_match_decision.down.sql

BEGIN;

DROP INDEX IF EXISTS smartorder.run_lines_offer_state;
DROP INDEX IF EXISTS smartorder.run_lines_match_state;

ALTER TABLE smartorder.run_lines DROP CONSTRAINT IF EXISTS run_lines_outcome_check;
ALTER TABLE smartorder.run_lines ADD CONSTRAINT run_lines_outcome_check
    CHECK (outcome IN ('ordered','no_supplier','coverage_blocked','institutional_blocked','out_of_stock','below_min_qty','unmatched','zero_qty','removed','quota_blocked'));

ALTER TABLE smartorder.run_lines
    DROP COLUMN IF EXISTS offer_state,
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
