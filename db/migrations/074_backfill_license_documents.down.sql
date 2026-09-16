-- 074_backfill_license_documents (down)
--
-- Restore the column and move commercial-register documents back onto it
-- (both the backfilled ones and any created by registration since). The
-- column carries the legacy Arabic comment from 047.

BEGIN;

ALTER TABLE org.organizations
    ADD COLUMN IF NOT EXISTS license_document_url TEXT NOT NULL DEFAULT ''; -- اتصال الملف الإداري الخاص بترخيص المنشأة / السجل التجاري

UPDATE org.organizations o
SET license_document_url = d.storage_key
FROM platform_admin.documents d
WHERE d.organization_id = o.id
  AND d.document_type = 'commercial_register'
  AND d.storage_key <> ''
  AND o.license_document_url = '';

DELETE FROM platform_admin.documents
WHERE document_type = 'commercial_register';

COMMIT;