-- 216_assistant_pending_actions.up.sql
--
-- Actions Capsule proposes and a person confirms.
--
-- The model can only create a 'pending' row. Moving it anywhere else takes a
-- separate, authenticated request from the user it belongs to: confirm in the
-- web drawer or on Telegram, or cancel. A row is single-use (the claim is a
-- compare-and-set from 'pending' to 'executing'), expires after ten minutes,
-- and carries the preview the person saw together with its hash, so a confirm
-- against changed data is refused as 'stale' instead of doing something the
-- person did not see.
--
-- args holds verified row ids. It never leaves the server: the model and the
-- browser only ever see public_id and the rendered preview.
BEGIN;

CREATE TABLE assistant.pending_actions (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id       UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT REFERENCES org.organizations(id) ON DELETE CASCADE,
    user_id         BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    scope           TEXT NOT NULL CHECK (scope IN ('pharmacy', 'vendor', 'admin')),
    conversation_id BIGINT REFERENCES assistant.conversations(id) ON DELETE SET NULL,
    channel         TEXT NOT NULL CHECK (channel IN ('web', 'telegram')),
    action          TEXT NOT NULL,
    risk            TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    args            JSONB NOT NULL,
    preview         JSONB NOT NULL,
    preview_hash    BYTEA NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'executing', 'executed', 'failed', 'cancelled', 'expired', 'stale')),
    outcome         JSONB,
    error_message   TEXT NOT NULL DEFAULT '',
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at      TIMESTAMPTZ,
    executed_at     TIMESTAMPTZ
);
CREATE INDEX pending_actions_user_idx ON assistant.pending_actions (user_id, created_at DESC);
CREATE INDEX pending_actions_open_idx ON assistant.pending_actions (expires_at) WHERE status = 'pending';

COMMENT ON TABLE assistant.pending_actions IS 'إجراءات يقترحها المساعد ولا تُنفَّذ إلا بتأكيد صريح من المستخدم نفسه؛ استخدام واحد وتنتهي خلال عشر دقائق';
COMMENT ON COLUMN assistant.pending_actions.args IS 'معطيات مُتحقق منها (معرّفات صفوف)؛ لا تُرسل للنموذج ولا للمتصفح';
COMMENT ON COLUMN assistant.pending_actions.preview_hash IS 'تجزئة المعاينة التي رآها المستخدم؛ التأكيد يُرفض إذا تغيرت البيانات';

ALTER TABLE assistant.pending_actions ENABLE ROW LEVEL SECURITY;
ALTER TABLE assistant.pending_actions FORCE ROW LEVEL SECURITY;
CREATE POLICY pending_actions_system_only ON assistant.pending_actions
    USING (platform.is_system()) WITH CHECK (platform.is_system());

COMMIT;
