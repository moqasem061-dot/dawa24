-- 070_merge_file_uploads (up)
--
-- Rebuild V2 §2.4d — ingest.file_uploads merges into platform_admin.documents
-- with document_type = 'import_file'. The legacy columns map 1:1 onto the
-- 049-expanded documents shape (filename → original_name, file_size_bytes →
-- size_bytes). import_sessions.file_upload_id re-points to documents.

BEGIN;

ALTER TABLE ingest.import_sessions
    DROP CONSTRAINT IF EXISTS import_sessions_file_upload_id_fkey;


-- Orphans must be cleared while file_upload_id still means file_uploads.id.
-- Doing it after the insert compares it against documents.id instead, so a
-- stale id that happens to match an unrelated document would be kept and
-- silently re-point the session at someone else's file.
UPDATE ingest.import_sessions s
SET file_upload_id = NULL
WHERE s.file_upload_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM ingest.file_uploads f WHERE f.id = s.file_upload_id);

-- documents.meta is NOT NULL DEFAULT '{}'::jsonb; an explicit NULL defeats the
-- default and violates the constraint.
INSERT INTO platform_admin.documents (
    public_id, organization_id, user_id, title, document_type, storage_key,
    original_name, file_url, status, mime_type, size_bytes, meta, created_at, updated_at
)
SELECT public_id, organization_id, user_id, filename, 'import_file', storage_key,
       filename, '', 'pending', mime_type, file_size_bytes, '{}'::jsonb, created_at, now()
FROM ingest.file_uploads;

-- Re-point sessions at the new rows. document_type scopes the join so a
-- pre-existing document sharing a storage_key cannot capture the reference.
UPDATE ingest.import_sessions s
SET file_upload_id = d.id
FROM ingest.file_uploads fu
JOIN platform_admin.documents d
  ON d.storage_key = fu.storage_key AND d.document_type = 'import_file'
WHERE s.file_upload_id = fu.id;

ALTER TABLE ingest.import_sessions
    ADD CONSTRAINT import_sessions_file_upload_id_fkey
    FOREIGN KEY (file_upload_id) REFERENCES platform_admin.documents(id) ON DELETE CASCADE;

DROP TABLE ingest.file_uploads;

COMMIT;

