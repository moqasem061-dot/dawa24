-- Migration 055: Job Seeker Profiles
-- Dedicated profiles for individual professionals (باحث عن عمل) seeking pharmacy/medical employment.

BEGIN;

CREATE TABLE IF NOT EXISTS hr.job_seeker_profiles (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    specialisation    TEXT NOT NULL DEFAULT 'pharmacist',
    years_experience  INT NOT NULL DEFAULT 0,
    cv_document_id    BIGINT REFERENCES platform_admin.documents(id) ON DELETE SET NULL,
    is_open_to_work   BOOLEAN NOT NULL DEFAULT true,
    expected_salary   NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    preferred_city_id BIGINT REFERENCES platform_admin.cities(id) ON DELETE SET NULL,
    bio               TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_job_seeker_user UNIQUE (user_id)
);

CREATE INDEX IF NOT EXISTS idx_job_seeker_specialisation ON hr.job_seeker_profiles (specialisation) WHERE is_open_to_work = true;
CREATE INDEX IF NOT EXISTS idx_job_seeker_city ON hr.job_seeker_profiles (preferred_city_id);

ALTER TABLE hr.job_seeker_profiles ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'job_seeker_profiles_policy') THEN
        CREATE POLICY job_seeker_profiles_policy ON hr.job_seeker_profiles
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

COMMIT;
