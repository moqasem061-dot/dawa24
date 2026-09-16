-- 038_user_address_histories
--
-- Append-only audit trail of every saved-address change (legacy
-- user_address_histories). Written in the same transaction as the change it
-- describes, so the timeline under /settings/addresses is always exact.

BEGIN;

CREATE TABLE IF NOT EXISTS identity.user_address_histories (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    address_id BIGINT,
    event      TEXT NOT NULL CHECK (event IN ('created','updated','deleted')),
    snapshot   JSONB NOT NULL DEFAULT '{}'::jsonb,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS user_address_histories_user_idx
    ON identity.user_address_histories (user_id, changed_at DESC);

COMMENT ON TABLE identity.user_address_histories IS
    'سجل تغييرات العناوين — append-only history of address changes';

COMMIT;
