-- Migration 025: HR Job Categories, Job Offers, and Applications
-- Schema: hr

CREATE TABLE IF NOT EXISTS hr.job_categories (
    id BIGSERIAL PRIMARY KEY,
    name JSONB NOT NULL,
    slug VARCHAR(64) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_job_categories_slug UNIQUE (slug)
);

CREATE TABLE IF NOT EXISTS hr.job_offers (
    id BIGSERIAL PRIMARY KEY,
    public_id VARCHAR(32) NOT NULL DEFAULT ('job_' || replace(gen_random_uuid()::text, '-', '')),
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    category_id BIGINT REFERENCES hr.job_categories(id) ON DELETE SET NULL,
    title JSONB NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    requirements TEXT NOT NULL DEFAULT '',
    salary_min NUMERIC(15,2),
    salary_max NUMERIC(15,2),
    location VARCHAR(128) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'published',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_job_offers_org ON hr.job_offers(organization_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_job_offers_category ON hr.job_offers(category_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS hr.job_applications (
    id BIGSERIAL PRIMARY KEY,
    public_id VARCHAR(32) NOT NULL DEFAULT ('app_' || replace(gen_random_uuid()::text, '-', '')),
    job_offer_id BIGINT NOT NULL REFERENCES hr.job_offers(id) ON DELETE CASCADE,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    applicant_name VARCHAR(128) NOT NULL,
    applicant_email VARCHAR(128) NOT NULL,
    applicant_phone VARCHAR(64) NOT NULL DEFAULT '',
    cv_storage_key VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_job_applications_offer ON hr.job_applications(job_offer_id);

ALTER TABLE hr.job_offers ENABLE ROW LEVEL SECURITY;
ALTER TABLE hr.job_offers FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_job_offers_isolation ON hr.job_offers
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id));

ALTER TABLE hr.job_applications ENABLE ROW LEVEL SECURITY;
ALTER TABLE hr.job_applications FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_job_applications_isolation ON hr.job_applications
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id));
