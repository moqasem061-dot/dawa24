-- 131_catalog_import_wizard (up)
--
-- The reviewed catalogue import gains an explicit mapping step and a durable
-- record of the run that follows it.
--
-- Two things were wrong with the previous shape. First, the analysis step wrote
-- no counts, so a session sitting at 'draft' reported nought rows, nought
-- products and nought blocks whatever the file actually held — the "it returns
-- zero results" the admin saw was the row, not the parser. Second, preparation
-- ran in a goroutine whose only record of itself lived in process memory: a
-- failure, a restart, or ten minutes of silence left the session at 'draft'
-- with an empty error_message and no way to tell a crashed run from one that
-- never started.
--
-- So: a 'processing' status the background run owns, the detected file
-- structure kept on the row so the mapping screen never re-reads a 32 MB
-- upload to draw itself, and the run's own phase persisted beside it.

BEGIN;

ALTER TABLE catalog.import_sessions
    DROP CONSTRAINT IF EXISTS import_sessions_status_check;

ALTER TABLE catalog.import_sessions
    ADD CONSTRAINT import_sessions_status_check
    CHECK (status IN ('draft','processing','ready','committing','committed','cancelled','failed'));

-- structure is the file as it was read: which row holds the titles, which
-- column is which field, and a few sample values per column. It is written once
-- at analysis and rewritten whenever the admin corrects the mapping, so the
-- mapping and review screens render from one small document instead of
-- decoding the workbook again on every request.
ALTER TABLE catalog.import_sessions
    ADD COLUMN IF NOT EXISTS structure JSONB NOT NULL DEFAULT '{}'::JSONB;

-- The background run's own progress. Memory-only progress is still the fast
-- path; these columns are what answers the poll after a deploy, a crash, or a
-- second browser tab that never saw the run start.
ALTER TABLE catalog.import_sessions
    ADD COLUMN IF NOT EXISTS progress_phase   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS progress_current INT  NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS progress_total   INT  NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS progress_at      TIMESTAMPTZ;

-- A run interrupted by a deploy leaves 'processing' behind. Nothing but a
-- living goroutine ever moves it forward, so anything stale is a failure the
-- admin must be told about rather than a screen that polls for ever. This
-- clears the ones already stranded by the previous shape.
UPDATE catalog.import_sessions
   SET status = 'failed',
       error_message = 'توقفت معالجة الملف قبل اكتمالها. يرجى إعادة المعالجة.'
 WHERE status = 'draft'
   AND created_at < now() - INTERVAL '1 hour'
   AND parsed_rows = 0;

COMMIT;
