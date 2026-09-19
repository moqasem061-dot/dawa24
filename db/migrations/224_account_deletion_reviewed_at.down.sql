-- 224_account_deletion_reviewed_at.down.sql

BEGIN;

ALTER TABLE identity.account_deletion_requests
    DROP COLUMN IF EXISTS reviewed_at;

COMMIT;
