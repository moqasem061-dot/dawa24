BEGIN;
ALTER TABLE platform_admin.contact_messages
  DROP COLUMN IF EXISTS ip,
  DROP COLUMN IF EXISTS user_agent,
  DROP COLUMN IF EXISTS user_id;
COMMIT;
