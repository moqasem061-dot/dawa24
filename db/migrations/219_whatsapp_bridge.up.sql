-- 219_whatsapp_bridge.up.sql
--
-- WhatsApp as a further interface to Capsule and to the in-app notification
-- feed, with exactly the shape of the telegram schema (migration 213) so both
-- channels share their statements (chatbridge/postgres). Nothing here decides
-- who may see what: identity, membership, roles and permissions are re-read on
-- every message.
--
-- A WhatsApp account is identified by its wa_id, the phone number in
-- international form without "+". last_inbound_at records the user's last
-- message, because WhatsApp only accepts free-form messages within 24 hours of
-- it; outside that window a notification must travel as an approved template.
BEGIN;

CREATE SCHEMA IF NOT EXISTS whatsapp;

CREATE TABLE whatsapp.link_tokens (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    organization_id BIGINT REFERENCES org.organizations(id) ON DELETE CASCADE,
    token_hash      BYTEA NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    consumed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX link_tokens_user_created_idx ON whatsapp.link_tokens (user_id, created_at DESC);

COMMENT ON TABLE whatsapp.link_tokens IS 'رموز ربط واتساب لمرة واحدة؛ يُخزَّن تجزئة الرمز فقط';

CREATE TABLE whatsapp.links (
    id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id              UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    user_id                BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    wa_id                  TEXT NOT NULL CHECK (wa_id ~ '^[0-9]{6,20}$'),
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
    last_inbound_at        TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX links_one_live_per_wa_id
    ON whatsapp.links (wa_id) WHERE status IN ('pending', 'active', 'blocked');
CREATE UNIQUE INDEX links_one_confirmed_per_user
    ON whatsapp.links (user_id) WHERE status IN ('active', 'blocked');
CREATE INDEX links_user_status_idx ON whatsapp.links (user_id, status);

COMMENT ON TABLE whatsapp.links IS 'ربط رقم واتساب بحساب مستخدم Dawa24؛ الصلاحيات لا تُخزَّن هنا وتُقرأ عند كل رسالة';
COMMENT ON COLUMN whatsapp.links.wa_id IS 'رقم واتساب بالصيغة الدولية بدون +';
COMMENT ON COLUMN whatsapp.links.status IS 'pending بانتظار التأكيد من الموقع، active مفعّل، blocked تعذّر التسليم نهائياً، revoked ملغى';
COMMENT ON COLUMN whatsapp.links.last_inbound_at IS 'آخر رسالة من المستخدم؛ تحدد نافذة الـ24 ساعة للرسائل الحرة';

-- WhatsApp may deliver a webhook more than once; each message is processed once.
CREATE TABLE whatsapp.processed_messages (
    message_id  TEXT PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX processed_messages_received_idx ON whatsapp.processed_messages (received_at);

CREATE TABLE whatsapp.deliveries (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    link_id             BIGINT NOT NULL REFERENCES whatsapp.links(id) ON DELETE CASCADE,
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
    ON whatsapp.deliveries (link_id, notification_log_id) WHERE notification_log_id IS NOT NULL;
CREATE INDEX deliveries_due_idx
    ON whatsapp.deliveries (next_attempt_at, id) WHERE status IN ('queued', 'leased');

COMMENT ON TABLE whatsapp.deliveries IS 'صندوق صادر رسائل واتساب الذي يسحبه n8n؛ الأهلية تحددها Dawa24';

ALTER TABLE whatsapp.link_tokens        ENABLE ROW LEVEL SECURITY;
ALTER TABLE whatsapp.link_tokens        FORCE ROW LEVEL SECURITY;
ALTER TABLE whatsapp.links              ENABLE ROW LEVEL SECURITY;
ALTER TABLE whatsapp.links              FORCE ROW LEVEL SECURITY;
ALTER TABLE whatsapp.processed_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE whatsapp.processed_messages FORCE ROW LEVEL SECURITY;
ALTER TABLE whatsapp.deliveries         ENABLE ROW LEVEL SECURITY;
ALTER TABLE whatsapp.deliveries         FORCE ROW LEVEL SECURITY;

CREATE POLICY system_only ON whatsapp.link_tokens
    USING (platform.is_system()) WITH CHECK (platform.is_system());
CREATE POLICY system_only ON whatsapp.links
    USING (platform.is_system()) WITH CHECK (platform.is_system());
CREATE POLICY system_only ON whatsapp.processed_messages
    USING (platform.is_system()) WITH CHECK (platform.is_system());
CREATE POLICY system_only ON whatsapp.deliveries
    USING (platform.is_system()) WITH CHECK (platform.is_system());

-- A proposal made on WhatsApp records its channel like one made on Telegram.
ALTER TABLE assistant.pending_actions DROP CONSTRAINT pending_actions_channel_check;
ALTER TABLE assistant.pending_actions ADD CONSTRAINT pending_actions_channel_check
    CHECK (channel IN ('web', 'telegram', 'whatsapp'));

COMMIT;
