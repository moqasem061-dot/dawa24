-- Migration 112: Advanced Product Import and Multi-Stage Matching Engine
ALTER TABLE ingest.import_sessions
    ADD COLUMN IF NOT EXISTS warehouse_id BIGINT NULL REFERENCES inventory.warehouses(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS import_mode VARCHAR(64) NOT NULL DEFAULT 'update_and_add',
    ADD COLUMN IF NOT EXISTS enable_ai_matching BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS enable_savings_matching BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS unmatched_rows INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS review_rows INT NOT NULL DEFAULT 0;

ALTER TABLE ingest.import_rows
    ADD COLUMN IF NOT EXISTS match_reason TEXT NULL,
    ADD COLUMN IF NOT EXISTS confidence_level VARCHAR(32) NOT NULL DEFAULT 'unmatched',
    ADD COLUMN IF NOT EXISTS candidate_matches JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS is_approved BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS import_action VARCHAR(32) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS error_details TEXT NULL;

CREATE INDEX IF NOT EXISTS idx_import_sessions_warehouse_id ON ingest.import_sessions(warehouse_id);
CREATE INDEX IF NOT EXISTS idx_import_rows_confidence_level ON ingest.import_rows(session_id, confidence_level);

