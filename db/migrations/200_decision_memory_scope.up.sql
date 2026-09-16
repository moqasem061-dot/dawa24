BEGIN;

-- 'org'      : written by and for one organisation (today's only behaviour)
-- 'platform' : promoted by an administrator; readable by every organisation
--              that has not opted out.
ALTER TABLE catalog.match_decisions
  ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'org'
    CHECK (scope IN ('org','platform')),
  ADD COLUMN IF NOT EXISTS promoted_by BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS promoted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'ai'
    CHECK (source IN ('ai','manual','admin','import'));

UPDATE catalog.match_decisions SET scope = 'platform' WHERE organization_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_match_decisions_scope_key
  ON catalog.match_decisions (scope, decision_key);
CREATE INDEX IF NOT EXISTS idx_match_decisions_org_lastused
  ON catalog.match_decisions (organization_id, last_used_at DESC);

-- Per-organisation opt-out of the platform-wide memory.
CREATE TABLE IF NOT EXISTS catalog.decision_memory_preferences (
  organization_id     BIGINT PRIMARY KEY REFERENCES org.organizations(id) ON DELETE CASCADE,
  use_platform_memory BOOLEAN NOT NULL DEFAULT true,
  updated_by          BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
