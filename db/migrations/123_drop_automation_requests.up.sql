-- 123_drop_automation_requests (up)
--
-- Removes workflow.automation_requests, the storage of the superseded
-- "Automatic Purchase Request" feature (Plan V5 Phase 3 Task 3.3).
--
-- The feature is replaced by Smart Ordering
-- (specs/001-smart-ordering-system), which is a five-step, resumable wizard
-- rather than a one-shot upload. The old table is dropped rather than migrated
-- because it held **zero rows** on the live database on 2026-08-25: nothing is
-- lost and there is nothing to carry across.
--
-- Migration 091 created this table and stays on disk untouched. The runner
-- checksums applied migrations and refuses altered history, so a table is
-- removed by adding a migration, never by editing or deleting the one that
-- created it.
--
-- Expand/contract note: the code reading this table is removed in the same
-- change. Rolling back to the previous image after this migration has run
-- would restore handlers whose queries fail — 123_drop_automation_requests.down.sql
-- recreates the table so that rollback stays viable.

BEGIN;

DROP TABLE IF EXISTS workflow.automation_requests CASCADE;

COMMIT;
