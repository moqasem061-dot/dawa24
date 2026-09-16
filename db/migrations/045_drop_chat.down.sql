-- 045_drop_chat (down)
BEGIN;

ALTER TABLE identity.users DROP COLUMN IF EXISTS avatar_url;

COMMIT;
