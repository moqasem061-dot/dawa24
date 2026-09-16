BEGIN;

DROP TABLE IF EXISTS workflow.report_issues CASCADE;
DROP TABLE IF EXISTS workflow.weekly_coverages CASCADE;
DROP TABLE IF EXISTS workflow.purchase_priority_engines CASCADE;

DROP SCHEMA IF EXISTS workflow CASCADE;

COMMIT;
