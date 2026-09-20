BEGIN;

DROP INDEX IF EXISTS hr.idx_job_offers_org_status;
DROP INDEX IF EXISTS hr.idx_job_applications_org;
DROP INDEX IF EXISTS hr.idx_job_applications_user;

COMMIT;
