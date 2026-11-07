-- 252_mfa_failure_counter.up.sql
-- Batch 5 (S20): second-factor guesses get their own failure counter. Before
-- this, a correct password reset the shared login counter, and the MFA step
-- counted against that same counter, so an attacker who knew the password could
-- alternate login and four wrong codes forever. The new columns default to zero
-- and null, so every existing account starts with a clean second-factor counter.

BEGIN;

ALTER TABLE identity.user_security
    ADD COLUMN IF NOT EXISTS mfa_failed_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS mfa_locked_until TIMESTAMPTZ;

COMMENT ON COLUMN identity.user_security.mfa_failed_attempts IS
    'Wrong second-factor (TOTP or recovery) codes since the last successful second factor. Separate from login_attempts, so a correct password does not clear it.';
COMMENT ON COLUMN identity.user_security.mfa_locked_until IS
    'Until when second-factor checks are refused for this account after too many wrong codes. Null when not locked.';

COMMIT;
