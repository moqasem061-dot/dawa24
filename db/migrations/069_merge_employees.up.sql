-- 069_merge_employees (up)
--
-- Rebuild V2 §2.4c — hr.employees folds onto org.members. The employee pay/HR
-- columns (employee_code, job_title, base_salary, variable_salary) already
-- live on org.members since 052; this migration adds public_id + hired_at and
-- moves the data. An employee becomes a member row; users who were employees
-- but not members are inserted with the org_pharmacist role (the legacy
-- employee row carried no role).

BEGIN;

ALTER TABLE org.members
    ADD COLUMN IF NOT EXISTS public_id UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN IF NOT EXISTS hired_at DATE;

COMMENT ON COLUMN org.members.public_id IS 'معرّف عام للعضو — موروث من hr.employees (069)';
COMMENT ON COLUMN org.members.hired_at IS 'تاريخ التعيين — موروث من hr.employees (069)';

-- Existing members take the legacy employee values (public_id preserved so
-- the HR API keeps returning the same public ids).
UPDATE org.members m
SET employee_code  = e.employee_code,
    job_title      = e.job_title,
    base_salary    = e.base_salary,
    variable_salary = e.variable_salary,
    status         = e.status,
    hired_at       = e.hired_at,
    public_id      = e.public_id
FROM hr.employees e
WHERE e.organization_id = m.organization_id
  AND e.user_id = m.user_id;

-- Employee rows with no member row become members (org_pharmacist role).
INSERT INTO org.members (organization_id, user_id, role_key, status, public_id,
                         employee_code, job_title, base_salary, variable_salary, hired_at)
SELECT e.organization_id, e.user_id, 'org_pharmacist', e.status, e.public_id,
       e.employee_code, e.job_title, e.base_salary, e.variable_salary, e.hired_at
FROM hr.employees e
WHERE NOT EXISTS (
    SELECT 1 FROM org.members m
    WHERE m.organization_id = e.organization_id AND m.user_id = e.user_id
);

-- Legacy employees_code_unique (organization_id, employee_code), non-empty
-- only: pre-052 members carry the '' default and must not collide.
CREATE UNIQUE INDEX IF NOT EXISTS members_employee_code_org_unique
    ON org.members (organization_id, employee_code)
    WHERE employee_code <> '';

DROP TABLE hr.employees;

COMMIT;
