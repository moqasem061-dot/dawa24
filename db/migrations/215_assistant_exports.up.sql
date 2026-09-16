-- 215_assistant_exports.up.sql
--
-- Spreadsheets Capsule generates with export_data. A file belongs to the user
-- and organisation it was generated for and is downloaded by a random token
-- whose SHA-256 is all that is stored here. Files expire after seven days; the
-- worker's assistant sweep deletes them.
BEGIN;

CREATE TABLE assistant.exports (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash      BYTEA NOT NULL UNIQUE,
    organization_id BIGINT REFERENCES org.organizations(id) ON DELETE CASCADE,
    user_id         BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    filename        TEXT NOT NULL,
    mime_type       TEXT NOT NULL,
    row_count       INTEGER NOT NULL CHECK (row_count >= 0),
    content         BYTEA NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX exports_expires_idx ON assistant.exports (expires_at);
CREATE INDEX exports_user_idx ON assistant.exports (user_id, created_at DESC);

COMMENT ON TABLE assistant.exports IS 'ملفات يصدّرها المساعد للمستخدم؛ تُحمَّل برمز عشوائي يُخزَّن تجزئته فقط وتنتهي بعد سبعة أيام';

ALTER TABLE assistant.exports ENABLE ROW LEVEL SECURITY;
ALTER TABLE assistant.exports FORCE ROW LEVEL SECURITY;
CREATE POLICY exports_system_only ON assistant.exports
    USING (platform.is_system()) WITH CHECK (platform.is_system());

COMMIT;
