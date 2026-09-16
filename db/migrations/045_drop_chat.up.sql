-- 045_drop_chat (up)
BEGIN;

-- Add avatar_url to identity.users
ALTER TABLE identity.users ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '';

-- Drop chat system tables and schema
DROP TABLE IF EXISTS chat.participants CASCADE;
DROP TABLE IF EXISTS chat.messages CASCADE;
DROP TABLE IF EXISTS chat.conversations CASCADE;
DROP SCHEMA IF EXISTS chat CASCADE;

COMMIT;
