-- 036_chat (down)
BEGIN;

DROP TABLE IF EXISTS chat.participants;
DROP TABLE IF EXISTS chat.messages;
DROP TABLE IF EXISTS chat.conversations;

-- The up migration creates the schema, so the down migration removes it.
DROP SCHEMA IF EXISTS chat;

COMMIT;
