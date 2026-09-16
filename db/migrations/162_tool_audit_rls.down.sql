-- 162_tool_audit_rls (down)

BEGIN;

DROP POLICY IF EXISTS tenant_isolation_tool_audit ON assistant.tool_audit;

ALTER TABLE assistant.tool_audit NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assistant.tool_audit DISABLE ROW LEVEL SECURITY;

COMMIT;
