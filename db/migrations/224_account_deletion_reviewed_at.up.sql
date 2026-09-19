-- 224_account_deletion_reviewed_at.up.sql
--
-- Ensure identity.account_deletion_requests contains both resolved_at and reviewed_at
-- columns with data synchronisation, preventing SQL 42703 column missing errors.

BEGIN;

ALTER TABLE identity.account_deletion_requests
    ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;

ALTER TABLE identity.account_deletion_requests
    ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;

UPDATE identity.account_deletion_requests
SET reviewed_at = resolved_at
WHERE reviewed_at IS NULL AND resolved_at IS NOT NULL;

UPDATE identity.account_deletion_requests
SET resolved_at = reviewed_at
WHERE resolved_at IS NULL AND reviewed_at IS NOT NULL;

COMMENT ON COLUMN identity.account_deletion_requests.reviewed_at IS 'تاريخ ووقت مراجعة الطلب — Timestamp when account deletion request was reviewed';
COMMENT ON COLUMN identity.account_deletion_requests.resolved_at IS 'تاريخ ووقت البت في الطلب — Timestamp when account deletion request was resolved';

COMMIT;
