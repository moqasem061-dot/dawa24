BEGIN;

-- 1. SQL Query Logs for developer diagnostic tracking
CREATE TABLE IF NOT EXISTS platform_admin.sql_logs (
    id BIGSERIAL PRIMARY KEY,
    query TEXT NOT NULL,
    executed_by BIGINT,
    actor_name TEXT NOT NULL DEFAULT '',
    duration_ms BIGINT NOT NULL DEFAULT 0,
    rows_affected BIGINT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS sql_logs_created_at_idx ON platform_admin.sql_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS sql_logs_executed_by_idx ON platform_admin.sql_logs (executed_by);

-- 2. Error Logs matching ported Laravel full_error_logs
CREATE TABLE IF NOT EXISTS platform_admin.error_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT,
    user_name TEXT NOT NULL DEFAULT '',
    user_email TEXT NOT NULL DEFAULT '',
    organization_name TEXT NOT NULL DEFAULT '',
    error_level TEXT NOT NULL DEFAULT 'ERROR',
    error_message TEXT NOT NULL,
    exception_class TEXT NOT NULL DEFAULT '',
    stack_trace TEXT NOT NULL DEFAULT '',
    file_path TEXT NOT NULL DEFAULT '',
    line_number INT NOT NULL DEFAULT 0,
    http_method TEXT NOT NULL DEFAULT 'GET',
    url_path TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'NEW',
    developer_notes TEXT NOT NULL DEFAULT '',
    fixed_by BIGINT,
    fixed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS error_logs_status_idx ON platform_admin.error_logs (status);
CREATE INDEX IF NOT EXISTS error_logs_created_at_idx ON platform_admin.error_logs (created_at DESC);

COMMIT;
