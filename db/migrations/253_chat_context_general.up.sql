-- 253_chat_context_general.up.sql
--
-- The chat module starts conversations with context_type 'general' (the
-- default in chat.ContextGeneral, and the inquiry flow in the UI). Migration
-- 049 rebuilt chat.conversations with a CHECK that does not list 'general', so
-- every "message a supplier" insert failed with SQLSTATE 23514. This widens the
-- CHECK to accept 'general'. Widening a CHECK is backward compatible: rows and
-- the old application code are unaffected.

BEGIN;

ALTER TABLE chat.conversations DROP CONSTRAINT IF EXISTS conversations_context_type_check;
ALTER TABLE chat.conversations ADD CONSTRAINT conversations_context_type_check
    CHECK (context_type IN ('direct', 'order', 'support', 'quote', 'general'));

COMMIT;
