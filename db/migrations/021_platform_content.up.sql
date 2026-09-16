-- Migration 021: Platform Content, Translations, Policies, System Resources, API Integrations
-- Schema: platform_admin

CREATE TABLE IF NOT EXISTS platform_admin.translations (
    id BIGSERIAL PRIMARY KEY,
    key CITEXT NOT NULL,
    translation_group VARCHAR(64) NOT NULL DEFAULT 'general',
    text JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_translations_key UNIQUE (key)
);

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

CREATE TABLE IF NOT EXISTS platform_admin.system_resources (
    id BIGSERIAL PRIMARY KEY,
    key VARCHAR(128) NOT NULL,
    name JSONB NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_system_resources_key UNIQUE (key)
);

CREATE TABLE IF NOT EXISTS platform_admin.api_integrations (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL,
    credentials JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_api_integrations_org_provider UNIQUE (organization_id, provider)
);

ALTER TABLE platform_admin.api_integrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_admin.api_integrations FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_api_integrations_isolation ON platform_admin.api_integrations
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id));
