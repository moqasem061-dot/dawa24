-- 157_compare_file_visibility.down.sql
BEGIN;

DROP INDEX IF EXISTS compare.compare_files_visibility_idx;

ALTER TABLE compare.files DROP COLUMN IF EXISTS visibility;

COMMIT;
