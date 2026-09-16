-- 187_compare_files_processing_status.up.sql
-- Add 'processing' status to compare.files status check constraint.
--
-- FileProcessing ("processing") is used when uploaded spreadsheets are being
-- read and staged in the background, but the existing files_status_check
-- constraint only permitted ('uploaded', 'mapping', 'ready', 'failed', 'archived').

ALTER TABLE compare.files DROP CONSTRAINT IF EXISTS files_status_check;

ALTER TABLE compare.files ADD CONSTRAINT files_status_check
    CHECK (status IN ('uploaded', 'processing', 'mapping', 'ready', 'failed', 'archived'));

COMMENT ON COLUMN compare.files.status IS 'حالة الملف (uploaded, processing, mapping, ready, failed, archived)';
