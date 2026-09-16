-- 074_backfill_license_documents (up)
--
-- Rebuild V2 §4.1 — registration documents survive as platform_admin.documents
-- rows and are never consumed. Backfill the legacy organizations
-- .license_document_url values into documents, then drop the column.
-- Approved organizations' files are marked verified (they survived approval);
-- pending ones stay pending for review.

BEGIN;

INSERT INTO platform_admin.documents (
    organization_id, title, document_type, storage_key, status, original_name, file_url
)
SELECT
    id,
    'السجل التجاري / الترخيص',
    'commercial_register',
    license_document_url,
    CASE WHEN status = 'approved' THEN 'verified' ELSE 'pending' END,
    '',
    ''
FROM org.organizations
WHERE license_document_url IS NOT NULL AND license_document_url <> '';

ALTER TABLE org.organizations DROP COLUMN IF EXISTS license_document_url;

COMMIT;