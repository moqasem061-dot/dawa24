-- 253_chat_context_general.down.sql
--
-- Restores the CHECK from migration 049. Rows written as 'general' are moved to
-- the table default 'direct' first, so the narrower CHECK can be re-added.

BEGIN;

UPDATE chat.conversations SET context_type = 'direct' WHERE context_type = 'general';

ALTER TABLE chat.conversations DROP CONSTRAINT IF EXISTS conversations_context_type_check;
ALTER TABLE chat.conversations ADD CONSTRAINT conversations_context_type_check
    CHECK (context_type IN ('direct', 'order', 'support', 'quote'));

COMMIT;
