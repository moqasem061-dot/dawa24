-- 187_compare_files_processing_status.down.sql

ALTER TABLE compare.files DROP CONSTRAINT IF EXISTS files_status_check;

ALTER TABLE compare.files ADD CONSTRAINT files_status_check
    CHECK (status IN ('uploaded', 'mapping', 'ready', 'failed', 'archived'));

COMMENT ON COLUMN compare.files.status IS 'حالة الملف (uploaded, mapping, ready, failed, archived)';
