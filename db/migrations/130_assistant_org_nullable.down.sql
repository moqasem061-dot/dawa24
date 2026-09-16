BEGIN;

ALTER TABLE assistant.conversations ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE assistant.messages ALTER COLUMN organization_id SET NOT NULL;

DROP POLICY IF EXISTS tenant_isolation_conversations ON assistant.conversations;
CREATE POLICY tenant_isolation_conversations ON assistant.conversations
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));

DROP POLICY IF EXISTS tenant_isolation_messages ON assistant.messages;
CREATE POLICY tenant_isolation_messages ON assistant.messages
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));

COMMIT;
