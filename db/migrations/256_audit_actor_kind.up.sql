BEGIN;
ALTER TABLE platform.audit_log
  ADD COLUMN IF NOT EXISTS actor_kind TEXT NOT NULL DEFAULT 'user',
  ADD COLUMN IF NOT EXISTS actor_key_id BIGINT;
ALTER TABLE platform.audit_log ADD CONSTRAINT audit_log_actor_kind_chk
  CHECK (actor_kind IN ('user','api_key','system'));
CREATE INDEX IF NOT EXISTS audit_log_actor_key_idx ON platform.audit_log (actor_key_id, created_at DESC) WHERE actor_key_id IS NOT NULL;
COMMENT ON COLUMN platform.audit_log.actor_kind IS 'نوع الفاعل: مستخدم أو مفتاح API أو النظام';
COMMENT ON COLUMN platform.audit_log.actor_key_id IS 'معرف مفتاح API عندما يكون الفاعل جهازاً';
COMMIT;
