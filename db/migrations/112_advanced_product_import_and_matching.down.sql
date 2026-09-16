-- Migration 112 Down
DROP INDEX IF EXISTS ingest.idx_import_rows_confidence_level;
DROP INDEX IF EXISTS ingest.idx_import_sessions_warehouse_id;

ALTER TABLE ingest.import_rows
    DROP COLUMN IF EXISTS error_details,
    DROP COLUMN IF EXISTS import_action,
    DROP COLUMN IF EXISTS is_approved,
    DROP COLUMN IF EXISTS candidate_matches,
    DROP COLUMN IF EXISTS confidence_level,
    DROP COLUMN IF EXISTS match_reason;

ALTER TABLE ingest.import_sessions
    DROP COLUMN IF EXISTS review_rows,
    DROP COLUMN IF EXISTS unmatched_rows,
    DROP COLUMN IF EXISTS enable_savings_matching,
    DROP COLUMN IF EXISTS enable_ai_matching,
    DROP COLUMN IF EXISTS import_mode,
    DROP COLUMN IF EXISTS warehouse_id;

