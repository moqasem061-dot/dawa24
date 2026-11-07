BEGIN;
DROP INDEX IF EXISTS platform.audit_log_actor_key_idx;
ALTER TABLE platform.audit_log DROP CONSTRAINT IF EXISTS audit_log_actor_kind_chk;
ALTER TABLE platform.audit_log DROP COLUMN IF EXISTS actor_key_id, DROP COLUMN IF EXISTS actor_kind;
COMMIT;
