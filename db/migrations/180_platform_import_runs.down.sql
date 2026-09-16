-- 180_platform_import_runs.down.sql

BEGIN;
DROP TRIGGER IF EXISTS trg_import_run_rows_updated_at ON platform.import_run_rows;
DROP TABLE IF EXISTS platform.import_run_rows CASCADE;
DROP TRIGGER IF EXISTS trg_import_runs_updated_at ON platform.import_runs;
DROP TABLE IF EXISTS platform.import_runs CASCADE;
COMMIT;
