-- 250_user_must_change_password.up.sql
-- Batch 3 (S06): accounts an administrator creates with a password of the
-- administrator's choosing must change it at first sign-in. The flag defaults to
-- false, so every existing account is unchanged. Sessions issued while it is set
-- are held on the change-password page until the password is changed.

BEGIN;

ALTER TABLE identity.users
    ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN identity.users.must_change_password IS
    'True when the account must set its own password at its next sign-in (new team members created with an administrator-chosen password).';

COMMIT;
