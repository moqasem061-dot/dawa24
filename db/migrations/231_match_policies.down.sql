-- 231_match_policies.down.sql

BEGIN;

DROP TABLE IF EXISTS catalog.match_policy_assignment_log;
DROP TABLE IF EXISTS catalog.match_policy_assignments;
DROP TABLE IF EXISTS catalog.match_policies;

COMMIT;
