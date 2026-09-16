-- 131_catalog_import_wizard (down)

BEGIN;

UPDATE catalog.import_sessions SET status = 'failed' WHERE status = 'processing';

ALTER TABLE catalog.import_sessions
    DROP CONSTRAINT IF EXISTS import_sessions_status_check;

ALTER TABLE catalog.import_sessions
    ADD CONSTRAINT import_sessions_status_check
    CHECK (status IN ('draft','committing','ready','committed','cancelled','failed'));

ALTER TABLE catalog.import_sessions
    DROP COLUMN IF EXISTS structure,
    DROP COLUMN IF EXISTS progress_phase,
    DROP COLUMN IF EXISTS progress_current,
    DROP COLUMN IF EXISTS progress_total,
    DROP COLUMN IF EXISTS progress_at;

COMMIT;
