-- Down Migration 047
DROP TABLE IF EXISTS platform_admin.policies;

ALTER TABLE org.organizations
    DROP COLUMN IF EXISTS license_document_url,
    DROP COLUMN IF EXISTS verification_notes,
    DROP COLUMN IF EXISTS rejection_reason,
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS approved_by;
