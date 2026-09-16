-- 119_catalog_import_ai_matched (up)
--
-- Records how many rows AI matching tied to an existing product.
--
-- It is worth its own column rather than being derived from the staging rows,
-- because those are deleted when a session is committed and the count is what
-- the import history shows afterwards: the number of duplicate catalogue
-- entries an import avoided creating is the clearest measure of what the
-- feature is for.

BEGIN;

ALTER TABLE catalog.import_sessions
    ADD COLUMN IF NOT EXISTS ai_matched INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN catalog.import_sessions.ai_matched IS
    'عدد الأصناف التي تم ربطها بأصناف موجودة عبر المطابقة الذكية بدلاً من تكرارها';

COMMIT;
