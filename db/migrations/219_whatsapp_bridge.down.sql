-- 219_whatsapp_bridge.down.sql
BEGIN;
DELETE FROM assistant.pending_actions WHERE channel = 'whatsapp';
ALTER TABLE assistant.pending_actions DROP CONSTRAINT pending_actions_channel_check;
ALTER TABLE assistant.pending_actions ADD CONSTRAINT pending_actions_channel_check
    CHECK (channel IN ('web', 'telegram'));
DROP SCHEMA IF EXISTS whatsapp CASCADE;
COMMIT;
