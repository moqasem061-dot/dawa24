-- 198_smartorder_quota_outcome (up)

ALTER TABLE smartorder.line_candidates DROP CONSTRAINT IF EXISTS line_candidates_ineligible_reason_check;
ALTER TABLE smartorder.line_candidates ADD CONSTRAINT line_candidates_ineligible_reason_check
    CHECK (ineligible_reason IS NULL OR ineligible_reason IN ('own_org','inactive','institutional','coverage','branch_no_location','stock','min_qty','quota'));

ALTER TABLE smartorder.run_lines DROP CONSTRAINT IF EXISTS run_lines_outcome_check;
ALTER TABLE smartorder.run_lines ADD CONSTRAINT run_lines_outcome_check
    CHECK (outcome IN ('ordered','no_supplier','coverage_blocked','institutional_blocked','out_of_stock','below_min_qty','unmatched','zero_qty','removed','quota_blocked'));

ALTER TABLE smartorder.runs ADD COLUMN IF NOT EXISTS quota_blocked_rows INT NOT NULL DEFAULT 0;
