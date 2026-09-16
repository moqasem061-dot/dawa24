-- 168_moderator_hierarchy.up.sql
-- Moderators (المشرفون) can now work under another moderator.
--
-- The platform's temporary warehouses are uploaded by staff moderators, and
-- until now every moderator was flat: they saw their own uploads and nothing
-- else, and only a super admin saw all of them. There was no way to say "these
-- four moderators report to that one, and that one should be able to see their
-- work" — which is how the operation is actually run.
--
-- The legacy system's answer was inventory.father_user_temparte_warehouses, a
-- side table keyed by user_id that nothing reads and that has never held a row.
-- The relationship is one column on the user, so it is one column on the user.
--
-- A NULL parent means a top-level moderator: they answer to the super admin and
-- may have moderators reporting to them. A non-NULL parent means they work
-- under that moderator, whose "مستودعات المشرفين تحت إدارتي" screen shows their
-- uploads.
--
-- Depth is deliberately one. Two levels is what the operation has; allowing a
-- chain would mean every access check walks a recursive query, and a cycle in
-- that graph is a hang. The check constraint below makes the self-reference
-- case impossible; the service refuses to parent a moderator who already has
-- children.

BEGIN;

ALTER TABLE identity.users
    ADD COLUMN IF NOT EXISTS moderator_parent_id BIGINT
        REFERENCES identity.users(id) ON DELETE SET NULL;

ALTER TABLE identity.users
    DROP CONSTRAINT IF EXISTS users_moderator_parent_not_self;
ALTER TABLE identity.users
    ADD CONSTRAINT users_moderator_parent_not_self
    CHECK (moderator_parent_id IS NULL OR moderator_parent_id <> id);

CREATE INDEX IF NOT EXISTS users_moderator_parent_idx
    ON identity.users (moderator_parent_id)
    WHERE moderator_parent_id IS NOT NULL;

COMMENT ON COLUMN identity.users.moderator_parent_id IS
    'المشرف الرئيسي الذي يتبعه هذا المشرف. NULL يعني مشرفاً رئيسياً مستقلاً يتبع مدير النظام مباشرة';

COMMIT;
