-- 250_user_must_change_password.down.sql

BEGIN;

ALTER TABLE identity.users DROP COLUMN IF EXISTS must_change_password;

COMMIT;
