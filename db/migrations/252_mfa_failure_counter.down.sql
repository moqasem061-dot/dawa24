-- 252_mfa_failure_counter.down.sql

BEGIN;

ALTER TABLE identity.user_security
    DROP COLUMN IF EXISTS mfa_locked_until,
    DROP COLUMN IF EXISTS mfa_failed_attempts;

COMMIT;
