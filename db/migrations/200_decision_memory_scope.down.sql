BEGIN;

DROP TABLE IF EXISTS catalog.decision_memory_preferences;

DROP INDEX IF EXISTS catalog.idx_match_decisions_org_lastused;
DROP INDEX IF EXISTS catalog.idx_match_decisions_scope_key;

ALTER TABLE catalog.match_decisions
  DROP COLUMN IF EXISTS source,
  DROP COLUMN IF EXISTS promoted_at,
  DROP COLUMN IF EXISTS promoted_by,
  DROP COLUMN IF EXISTS scope;

COMMIT;
