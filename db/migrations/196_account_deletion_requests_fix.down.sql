DROP INDEX IF EXISTS identity.account_deletion_requests_one_pending;

ALTER TABLE identity.account_deletion_requests
    DROP CONSTRAINT IF EXISTS account_deletion_requests_status_check;

ALTER TABLE identity.account_deletion_requests
    ADD CONSTRAINT account_deletion_requests_status_check
    CHECK (status IN ('under_review', 'approved', 'rejected', 'deleted', 'recovered'));