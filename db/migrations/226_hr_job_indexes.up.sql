BEGIN;

-- Add performance indexes for HR job offers and job applications
CREATE INDEX IF NOT EXISTS idx_job_offers_org_status ON hr.job_offers(organization_id, status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_job_applications_org ON hr.job_applications(organization_id);
CREATE INDEX IF NOT EXISTS idx_job_applications_user ON hr.job_applications(applicant_user_id);

COMMIT;
