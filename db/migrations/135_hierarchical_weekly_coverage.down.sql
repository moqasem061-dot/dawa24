-- 135_hierarchical_weekly_coverage.down.sql
DROP INDEX IF EXISTS workflow.idx_weekly_coverages_branch_day;
DROP INDEX IF EXISTS workflow.idx_weekly_coverages_gov_city;
DROP INDEX IF EXISTS workflow.idx_weekly_coverages_org_day_active;

ALTER TABLE workflow.weekly_coverages
    DROP COLUMN IF EXISTS governorate_id;
