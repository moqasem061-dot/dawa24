-- Why a tool call ended the way it did.
--
-- assistant.tool_audit recorded the decision ('allowed', 'denied_permission',
-- 'failed') and nothing about the reason. Read back from the live database,
-- that produced exactly the ambiguity the assistant work order warns about:
-- two 'failed' rows for my_offers, with no way to tell a timeout from a
-- refused permission from a query that broke, and no way to correlate them
-- with the application log except by timestamp.
--
-- detail carries a short, sanitized CLASS of reason -- 'timeout', 'canceled',
-- 'read_failed', 'unknown_field', a permission key. It must never carry a SQL
-- string, a table or column name, or another tenant's data: this column is
-- read by the platform conversation-audit screen, and it is written on a path
-- that a model's arguments can influence.
ALTER TABLE assistant.tool_audit
    ADD COLUMN IF NOT EXISTS detail text NOT NULL DEFAULT '';

COMMENT ON COLUMN assistant.tool_audit.detail IS
    'Sanitized reason class for the decision. Never SQL, table names, or tenant data.';
