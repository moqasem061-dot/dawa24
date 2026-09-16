-- Rollback migration 103: Drop assistant conversations and messages
DROP TABLE IF EXISTS assistant.messages CASCADE;
DROP TABLE IF EXISTS assistant.conversations CASCADE;
DROP SCHEMA IF EXISTS assistant CASCADE;
