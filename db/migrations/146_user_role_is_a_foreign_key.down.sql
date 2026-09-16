-- Reverse 146_user_role_is_a_foreign_key.
--
-- Restores the literal CHECK from migration 060. Roles outside that list are
-- normalised to 'user' first, because a rollback that leaves rows violating
-- the constraint it just added would fail on the next write instead of here.

BEGIN;

DROP INDEX IF EXISTS identity.idx_users_role;

ALTER TABLE identity.users DROP CONSTRAINT IF EXISTS users_role_fkey;

UPDATE identity.users
   SET role = 'user'
 WHERE role NOT IN ('user', 'support', 'admin', 'super_admin', 'developer');

ALTER TABLE identity.users ALTER COLUMN role SET DEFAULT 'customer';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'users_role_check' AND conrelid = 'identity.users'::regclass
    ) THEN
        ALTER TABLE identity.users ADD CONSTRAINT users_role_check
            CHECK (role IN ('user', 'support', 'admin', 'super_admin', 'developer'));
    END IF;
END $$;

COMMIT;
