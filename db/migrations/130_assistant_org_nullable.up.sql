-- 130_assistant_org_nullable
-- Allow assistant conversations and messages for system/staff users without mandatory organization binding,
-- and update RLS policies accordingly.

BEGIN;

ALTER TABLE assistant.conversations ALTER COLUMN organization_id DROP NOT NULL;
ALTER TABLE assistant.messages ALTER COLUMN organization_id DROP NOT NULL;

DROP POLICY IF EXISTS tenant_isolation_conversations ON assistant.conversations;
CREATE POLICY tenant_isolation_conversations ON assistant.conversations
    AS RESTRICTIVE
    USING (platform.is_system() OR (organization_id IS NOT NULL AND platform.tenant_visible(organization_id)))
    WITH CHECK (platform.is_system() OR (organization_id IS NOT NULL AND platform.tenant_visible(organization_id)));

DROP POLICY IF EXISTS tenant_isolation_messages ON assistant.messages;
CREATE POLICY tenant_isolation_messages ON assistant.messages
    AS RESTRICTIVE
    USING (platform.is_system() OR (organization_id IS NOT NULL AND platform.tenant_visible(organization_id)))
    WITH CHECK (platform.is_system() OR (organization_id IS NOT NULL AND platform.tenant_visible(organization_id)));

COMMIT;
