-- 232_match_runs.down.sql

BEGIN;

DROP POLICY IF EXISTS match_runs_tenant_isolation ON catalog.match_runs;
DROP TABLE IF EXISTS catalog.match_runs;

COMMIT;
