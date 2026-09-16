-- 116_catalog_import_source_file (up)
--
-- The wizard lets the admin correct the column mapping and the row range and
-- then run the file again, which means the file has to outlive the upload
-- request. Keeping the bytes on the session is what makes the review step
-- re-runnable without asking the admin to upload the same 9,000-row workbook a
-- second time.
--
-- It is stored on the row rather than in object storage deliberately: these
-- files are small (the real distributor catalogue is 350 KB), they are reaped
-- within 24 hours by the same sweep that collects abandoned sessions, and
-- putting them here keeps the import working on a deployment that has no S3
-- bucket configured.

BEGIN;

ALTER TABLE catalog.import_sessions
    ADD COLUMN IF NOT EXISTS source_file BYTEA NOT NULL DEFAULT ''::bytea;

COMMENT ON COLUMN catalog.import_sessions.source_file IS
    'الملف المرفوع، محفوظ مؤقتاً لإعادة المعالجة بعد تعديل الأعمدة، ويُحذف مع انتهاء صلاحية الجلسة';

COMMIT;
