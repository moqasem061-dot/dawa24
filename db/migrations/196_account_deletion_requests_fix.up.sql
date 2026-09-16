-- Migration 196: Account Deletion Requests Fix & Single Pending Constraint
--
-- Fixes check constraint to permit 'pending' and ensures users can only submit
-- one pending/under_review deletion request at a time.

ALTER TABLE identity.account_deletion_requests
    DROP CONSTRAINT IF EXISTS account_deletion_requests_status_check;

ALTER TABLE identity.account_deletion_requests
    ADD CONSTRAINT account_deletion_requests_status_check
    CHECK (status IN ('pending', 'under_review', 'approved', 'rejected', 'deleted', 'recovered'));

CREATE UNIQUE INDEX IF NOT EXISTS account_deletion_requests_one_pending
    ON identity.account_deletion_requests (user_id)
    WHERE status IN ('pending', 'under_review');