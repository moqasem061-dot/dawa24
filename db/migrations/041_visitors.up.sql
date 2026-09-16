-- 041_visitors
--
-- Visitor analytics (legacy visitors). One row per visitor session per day —
-- never per request, or the table grows unusably. IP is truncated to /24 and
-- that fact is disclosed in the privacy policy.

BEGIN;

CREATE TABLE IF NOT EXISTS platform_admin.visitors (
    id          BIGSERIAL PRIMARY KEY,
    visitor_key VARCHAR(64) NOT NULL,
    ip          VARCHAR(64) NOT NULL DEFAULT '',
    user_agent  TEXT NOT NULL DEFAULT '',
    browser     VARCHAR(32) NOT NULL DEFAULT '',
    device      VARCHAR(32) NOT NULL DEFAULT '',
    os          VARCHAR(32) NOT NULL DEFAULT '',
    country     VARCHAR(64) NOT NULL DEFAULT '',
    city        VARCHAR(64) NOT NULL DEFAULT '',
    visited_at  DATE NOT NULL DEFAULT CURRENT_DATE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_visitors_key_day UNIQUE (visitor_key, visited_at)
);

CREATE INDEX IF NOT EXISTS visitors_visited_at_idx ON platform_admin.visitors (visited_at DESC);

COMMENT ON TABLE platform_admin.visitors IS 'تحليلات الزوار — one row per visitor session per day, IP truncated to /24';

COMMIT;
