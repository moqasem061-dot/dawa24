-- 168_moderator_hierarchy.down.sql

BEGIN;

DROP INDEX IF EXISTS identity.users_moderator_parent_idx;
ALTER TABLE identity.users DROP CONSTRAINT IF EXISTS users_moderator_parent_not_self;
ALTER TABLE identity.users DROP COLUMN IF EXISTS moderator_parent_id;

COMMIT;
