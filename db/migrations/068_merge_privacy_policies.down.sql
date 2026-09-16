-- 068_merge_privacy_policies (down)
--
-- Recreate platform_admin.privacy_policies and move the tagged rows back.

BEGIN;

CREATE TABLE IF NOT EXISTS platform_admin.privacy_policies (
    id BIGSERIAL PRIMARY KEY,
    slug VARCHAR(64) NOT NULL,
    title JSONB NOT NULL,
    content JSONB NOT NULL,
    is_published BOOLEAN NOT NULL DEFAULT true,
    version INT NOT NULL DEFAULT 1,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_privacy_policies_slug_version UNIQUE (slug, version)
);

INSERT INTO platform_admin.privacy_policies (
    slug, title, content, is_published, version, published_at, created_at, updated_at
)
SELECT policy_key, title, content, is_published, version::INT, published_at, created_at, updated_at
FROM platform_admin.policies
WHERE policy_type = 'privacy';

ALTER TABLE platform_admin.policies
    DROP COLUMN IF EXISTS policy_type;

COMMIT;
