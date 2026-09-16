BEGIN;

-- Add organization_id and user_id to catalog.match_decisions
ALTER TABLE catalog.match_decisions
    ADD COLUMN IF NOT EXISTS organization_id BIGINT REFERENCES org.organizations(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES identity.users(id) ON DELETE SET NULL;

-- Drop old global unique index on decision_key if exists
DROP INDEX IF EXISTS catalog.match_decisions_key_uk;

-- Create composite unique index per tenant organization (using COALESCE to handle legacy global rows if any)
CREATE UNIQUE INDEX IF NOT EXISTS match_decisions_org_key_uk 
    ON catalog.match_decisions (COALESCE(organization_id, 0), decision_key);

CREATE INDEX IF NOT EXISTS match_decisions_org_idx 
    ON catalog.match_decisions (organization_id);

CREATE INDEX IF NOT EXISTS match_decisions_user_idx 
    ON catalog.match_decisions (user_id);

-- Add default platform setting for global decision memory switch
INSERT INTO platform_admin.system_settings (key, value, description, is_public, updated_at)
VALUES (
    'decision_memory_enabled',
    'true'::jsonb,
    'Global switch to enable or disable AI Decision Memory across all platform features',
    true,
    now()
) ON CONFLICT (key) DO NOTHING;

COMMIT;
