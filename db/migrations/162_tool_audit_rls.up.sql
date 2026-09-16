-- 162_tool_audit_rls
--
-- Row-level security for assistant.tool_audit.
--
-- This belongs logically in 160, and was briefly written there — which is
-- exactly the mistake the runner's checksum exists to catch. 160 had already
-- been applied to production by the time the policy was added, so the edited
-- file no longer matched its recorded hash and every subsequent deploy refused
-- to migrate at all. The rule holds without exception: an applied migration is
-- immutable, and a correction is a new file.
--
-- The policy itself: the audit log carries the same isolation as the reads it
-- describes. A row with no organisation belongs to platform staff, who have no
-- tenant, and is admitted only under the system flag — which is what the
-- application sets for exactly that case (assistant/postgres/support.go,
-- ownCtx). A tenant therefore sees only its own refusals, and never learns that
-- another organisation exists from this table.

BEGIN;

ALTER TABLE assistant.tool_audit ENABLE ROW LEVEL SECURITY;
ALTER TABLE assistant.tool_audit FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_tool_audit ON assistant.tool_audit;
CREATE POLICY tenant_isolation_tool_audit ON assistant.tool_audit
    AS RESTRICTIVE
    USING (platform.is_system() OR (organization_id IS NOT NULL AND platform.tenant_visible(organization_id)))
    WITH CHECK (platform.is_system() OR (organization_id IS NOT NULL AND platform.tenant_visible(organization_id)));

COMMIT;
