-- 146_user_role_is_a_foreign_key
--
-- identity.users.role was constrained by a CHECK listing five literal role
-- names. That made the whole point of a role editor impossible: a super admin
-- could create "Finance Moderator" and then not assign it to anybody, because
-- the row would fail a constraint written in migration 060. The failure
-- surfaces as a raw SQLSTATE 23514 at the moment of assignment, long after the
-- role appeared to have been created successfully.
--
-- Replacing the CHECK with a foreign key onto identity.roles moves the
-- question from "is this one of five names somebody typed in 2026" to "is this
-- a role that exists" — which is the question that was always meant.
--
-- The column default is corrected in the same change. It was 'customer', a
-- value the CHECK itself rejected, so any insert relying on the default failed.
-- Every code path happened to set the column explicitly, which is why nobody
-- noticed.

BEGIN;

-- The ordinary account role has to exist before anything can reference it.
-- The catalogue sync writes this row too, but that runs after migrations —
-- and the foreign key below cannot wait for it.
INSERT INTO identity.roles (key, name, scope, is_system, is_staff, description)
VALUES ('user',
        '{"ar":"مستخدم","en":"User"}'::JSONB,
        'platform', true, false,
        'حساب عادي؛ صلاحياته داخل لوحة المنشأة تأتي من عضويته وليس من دوره على المنصة.')
ON CONFLICT (key) DO NOTHING;

-- Any value not backed by a role row is normalised to the ordinary account
-- role first, or the foreign key cannot be created. There should be none:
-- migration 060 did the same normalisation for the CHECK.
UPDATE identity.users u
   SET role = 'user'
 WHERE NOT EXISTS (SELECT 1 FROM identity.roles r WHERE r.key = u.role);

ALTER TABLE identity.users DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE identity.users ALTER COLUMN role SET DEFAULT 'user';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'users_role_fkey'
    ) THEN
        -- ON UPDATE CASCADE so renaming a role key carries its holders with
        -- it. RESTRICT on delete: a role still held by an account must be
        -- reassigned deliberately, not silently emptied — the service refuses
        -- the delete with the headcount, which is a message an operator can
        -- act on.
        ALTER TABLE identity.users
            ADD CONSTRAINT users_role_fkey
            FOREIGN KEY (role) REFERENCES identity.roles (key)
            ON UPDATE CASCADE ON DELETE RESTRICT;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_users_role ON identity.users (role) WHERE deleted_at IS NULL;

COMMENT ON COLUMN identity.users.role IS
    'الدور على مستوى المنصة (identity.roles). صلاحيات العضو داخل منشأته تأتي من عضويته وليس من هنا.';

COMMIT;
