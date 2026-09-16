-- 213_telegram_bridge.up.sql
--
-- Telegram as a second interface to Capsule and to the in-app notification
-- feed. Nothing here decides who may see what: identity, membership, roles and
-- permissions stay where they are and are re-read on every message. These
-- tables only remember which Telegram account belongs to which Dawa24 user,
-- and which notifications have already been handed to n8n for delivery.
--
-- None of these rows belongs to a tenant: a link is a user's, and a user may
-- belong to several منشآت. Every table is therefore closed to everything but
-- system callers; the repository reaches them through database.AsSystem and
-- scopes each statement by user explicitly.
BEGIN;

CREATE SCHEMA IF NOT EXISTS telegram;

-- One-time link codes. Only the SHA-256 of the code is stored, so a database
-- read cannot be turned into a working link.
CREATE TABLE telegram.link_tokens (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    organization_id BIGINT REFERENCES org.organizations(id) ON DELETE CASCADE,
    token_hash      BYTEA NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    consumed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX link_tokens_user_created_idx ON telegram.link_tokens (user_id, created_at DESC);

COMMENT ON TABLE telegram.link_tokens IS 'رموز ربط تيليجرام لمرة واحدة؛ يُخزَّن تجزئة الرمز فقط';
COMMENT ON COLUMN telegram.link_tokens.organization_id IS 'المنشأة النشطة لحظة إنشاء الرمز، تصبح المنشأة الافتراضية للربط';

CREATE TABLE telegram.links (
    id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id              UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    user_id                BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    telegram_user_id       BIGINT NOT NULL,
    chat_id                BIGINT NOT NULL,
    username               TEXT NOT NULL DEFAULT '',
    display_name           TEXT NOT NULL DEFAULT '',
    status                 TEXT NOT NULL CHECK (status IN ('pending', 'active', 'blocked', 'revoked')),
    active_organization_id BIGINT REFERENCES org.organizations(id) ON DELETE SET NULL,
    conversation_id        BIGINT REFERENCES assistant.conversations(id) ON DELETE SET NULL,
    muted_categories       TEXT[] NOT NULL DEFAULT '{}',
    confirm_expires_at     TIMESTAMPTZ,
    confirmed_at           TIMESTAMPTZ,
    revoked_at             TIMESTAMPTZ,
    busy_until             TIMESTAMPTZ,
    last_seen_at           TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A Telegram account speaks for at most one Dawa24 user at a time, and a
-- Dawa24 user has at most one confirmed Telegram account.
CREATE UNIQUE INDEX links_one_live_per_telegram_user
    ON telegram.links (telegram_user_id) WHERE status IN ('pending', 'active', 'blocked');
CREATE UNIQUE INDEX links_one_confirmed_per_user
    ON telegram.links (user_id) WHERE status IN ('active', 'blocked');
CREATE INDEX links_user_status_idx ON telegram.links (user_id, status);

COMMENT ON TABLE telegram.links IS 'ربط حساب تيليجرام بحساب مستخدم Dawa24؛ الصلاحيات لا تُخزَّن هنا وتُقرأ عند كل رسالة';
COMMENT ON COLUMN telegram.links.status IS 'pending بانتظار التأكيد من الموقع، active مفعّل، blocked حظر المستخدم البوت، revoked ملغى';
COMMENT ON COLUMN telegram.links.active_organization_id IS 'المنشأة التي يعمل المساعد ضمنها في تيليجرام؛ يُتحقق من العضوية عند كل رسالة';
COMMENT ON COLUMN telegram.links.muted_categories IS 'فئات الإشعارات التي أوقفها المستخدم على تيليجرام';
COMMENT ON COLUMN telegram.links.busy_until IS 'قفل سؤال واحد في كل مرة لكل محادثة';

-- Telegram may deliver an update more than once; each is processed once.
CREATE TABLE telegram.processed_updates (
    update_id   BIGINT PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX processed_updates_received_idx ON telegram.processed_updates (received_at);

-- The outbox n8n drains. A row records the decision for one notification and
-- one link, including the decision not to send it ('dropped'), so the same
-- notification is never evaluated twice.
CREATE TABLE telegram.deliveries (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    link_id             BIGINT NOT NULL REFERENCES telegram.links(id) ON DELETE CASCADE,
    notification_log_id BIGINT REFERENCES notifications.logs(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL CHECK (kind IN ('notification', 'system')),
    category            TEXT NOT NULL DEFAULT 'general',
    text                TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'queued'
                        CHECK (status IN ('queued', 'leased', 'sent', 'failed', 'dropped')),
    drop_reason         TEXT NOT NULL DEFAULT '',
    attempts            INT NOT NULL DEFAULT 0,
    next_attempt_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_until         TIMESTAMPTZ,
    last_error          TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at             TIMESTAMPTZ
);
CREATE UNIQUE INDEX deliveries_once_per_notification
    ON telegram.deliveries (link_id, notification_log_id) WHERE notification_log_id IS NOT NULL;
CREATE INDEX deliveries_due_idx
    ON telegram.deliveries (next_attempt_at, id) WHERE status IN ('queued', 'leased');

COMMENT ON TABLE telegram.deliveries IS 'صندوق صادر رسائل تيليجرام الذي يسحبه n8n؛ الأهلية تحددها Dawa24';

ALTER TABLE telegram.link_tokens       ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram.link_tokens       FORCE ROW LEVEL SECURITY;
ALTER TABLE telegram.links             ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram.links             FORCE ROW LEVEL SECURITY;
ALTER TABLE telegram.processed_updates ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram.processed_updates FORCE ROW LEVEL SECURITY;
ALTER TABLE telegram.deliveries        ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram.deliveries        FORCE ROW LEVEL SECURITY;

CREATE POLICY system_only ON telegram.link_tokens
    USING (platform.is_system()) WITH CHECK (platform.is_system());
CREATE POLICY system_only ON telegram.links
    USING (platform.is_system()) WITH CHECK (platform.is_system());
CREATE POLICY system_only ON telegram.processed_updates
    USING (platform.is_system()) WITH CHECK (platform.is_system());
CREATE POLICY system_only ON telegram.deliveries
    USING (platform.is_system()) WITH CHECK (platform.is_system());

COMMIT;
