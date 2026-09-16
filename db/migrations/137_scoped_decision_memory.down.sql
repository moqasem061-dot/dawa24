BEGIN;

DROP INDEX IF EXISTS catalog.match_decisions_org_key_uk;
DROP INDEX IF EXISTS catalog.match_decisions_org_idx;
DROP INDEX IF EXISTS catalog.match_decisions_user_idx;

CREATE UNIQUE INDEX IF NOT EXISTS match_decisions_key_uk ON catalog.match_decisions (decision_key);

ALTER TABLE catalog.match_decisions
    DROP COLUMN IF EXISTS organization_id,
    DROP COLUMN IF EXISTS user_id;

DELETE FROM platform_admin.system_settings WHERE key = 'decision_memory_enabled';

COMMIT;
