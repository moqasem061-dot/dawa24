-- Migration 022: Ingest Batches and Progress Tracking
-- Schema: ingest

CREATE TABLE IF NOT EXISTS ingest.import_batches (
    id BIGSERIAL PRIMARY KEY,
    public_id VARCHAR(32) NOT NULL DEFAULT ('bat_' || replace(gen_random_uuid()::text, '-', '')),
    session_id BIGINT NOT NULL REFERENCES ingest.import_sessions(id) ON DELETE CASCADE,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    batch_number INT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    total_rows INT NOT NULL DEFAULT 0,
    processed_rows INT NOT NULL DEFAULT 0,
    matched_rows INT NOT NULL DEFAULT 0,
    error_rows INT NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_import_batches_session_batch UNIQUE (session_id, batch_number)
);

CREATE TABLE IF NOT EXISTS ingest.import_progress (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES ingest.import_sessions(id) ON DELETE CASCADE,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    percent INT NOT NULL DEFAULT 0,
    current_step VARCHAR(64) NOT NULL DEFAULT 'initialized',
    message TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_import_progress_session UNIQUE (session_id)
);

ALTER TABLE ingest.import_batches ENABLE ROW LEVEL SECURITY;
ALTER TABLE ingest.import_batches FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_import_batches_isolation ON ingest.import_batches
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id));

ALTER TABLE ingest.import_progress ENABLE ROW LEVEL SECURITY;
ALTER TABLE ingest.import_progress FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_import_progress_isolation ON ingest.import_progress
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id));
