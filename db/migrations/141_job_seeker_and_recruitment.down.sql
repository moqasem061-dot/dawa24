-- Migration 141: Down

ALTER TABLE hr.job_applications DROP COLUMN IF EXISTS branch_id;
ALTER TABLE hr.job_applications DROP COLUMN IF EXISTS assigned_role_key;
DROP TABLE IF EXISTS hr.job_seeker_profiles;
