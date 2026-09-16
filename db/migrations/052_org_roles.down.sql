-- Migration 052 Down

BEGIN;

ALTER TABLE org.members
    DROP COLUMN IF EXISTS employee_code,
    DROP COLUMN IF EXISTS job_title,
    DROP COLUMN IF EXISTS base_salary,
    DROP COLUMN IF EXISTS variable_salary,
    DROP COLUMN IF EXISTS org_role_id;

DROP TABLE IF EXISTS org.role_permissions;
DROP TABLE IF EXISTS org.roles;

COMMIT;
