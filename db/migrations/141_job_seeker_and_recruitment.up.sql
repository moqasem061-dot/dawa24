-- Migration 141: Job Seeker Account Type and Recruitment Onboarding
-- Schema: identity, hr, org

-- 1. Ensure platform role 'job_seeker' exists in identity.roles
INSERT INTO identity.roles (key, name, scope, is_system, description) VALUES
    ('job_seeker', '{"ar":"باحث عن عمل","en":"Job Seeker"}', 'platform', true, 'Individual medical & pharma professional seeking employment')
ON CONFLICT (key) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

-- 2. Enhance hr.job_applications with branch and assigned role for employee onboarding
ALTER TABLE hr.job_applications ADD COLUMN IF NOT EXISTS branch_id BIGINT REFERENCES org.branches(id) ON DELETE SET NULL;
ALTER TABLE hr.job_applications ADD COLUMN IF NOT EXISTS assigned_role_key TEXT REFERENCES identity.roles(key) ON DELETE SET NULL;

-- 3. Ensure hr.job_seeker_profiles table exists
CREATE TABLE IF NOT EXISTS hr.job_seeker_profiles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES identity.users(id) ON DELETE CASCADE,
    specialisation VARCHAR(64) NOT NULL DEFAULT '',
    years_experience INT NOT NULL DEFAULT 0,
    cv_document_id BIGINT,
    is_open_to_work BOOLEAN NOT NULL DEFAULT true,
    expected_salary NUMERIC(15,2) NOT NULL DEFAULT 0,
    preferred_city_id BIGINT REFERENCES platform_admin.cities(id) ON DELETE SET NULL,
    bio TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_job_seeker_profiles_user ON hr.job_seeker_profiles(user_id);
CREATE INDEX IF NOT EXISTS idx_job_seeker_profiles_spec ON hr.job_seeker_profiles(specialisation) WHERE is_open_to_work;
CREATE INDEX IF NOT EXISTS idx_job_seeker_profiles_city ON hr.job_seeker_profiles(preferred_city_id) WHERE is_open_to_work;
