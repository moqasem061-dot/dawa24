BEGIN;

-- 1. Create org.employee_institutional_works (Plan V5 Phase 1 Task 1.1)
CREATE TABLE IF NOT EXISTS org.employee_institutional_works (
    id                    BIGSERIAL PRIMARY KEY,
    organization_id       BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    user_id               BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    institutional_work_id BIGINT NOT NULL REFERENCES org.institutional_works(id) ON DELETE CASCADE,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at            TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_emp_inst_works_unique 
    ON org.employee_institutional_works (user_id, institutional_work_id) 
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_emp_inst_works_org 
    ON org.employee_institutional_works (organization_id) 
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_emp_inst_works_user 
    ON org.employee_institutional_works (user_id) 
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_emp_inst_works_work 
    ON org.employee_institutional_works (institutional_work_id) 
    WHERE deleted_at IS NULL;

-- Enable Row-Level Security
ALTER TABLE org.employee_institutional_works ENABLE ROW LEVEL SECURITY;

-- RLS Policy (Rule R8)
DROP POLICY IF EXISTS emp_inst_works_tenant_isolation ON org.employee_institutional_works;
CREATE POLICY emp_inst_works_tenant_isolation ON org.employee_institutional_works
    USING (
        organization_id = NULLIF(current_setting('app.current_tenant', true), '')::BIGINT
        OR current_setting('app.is_system', true) = 'true'
    );

COMMENT ON TABLE org.employee_institutional_works IS 'ربط موظفي وأعضاء المنشآت بمجموعات العمل المؤسسية لتحديد صلاحيات الرؤية';
COMMENT ON COLUMN org.employee_institutional_works.organization_id IS 'معرف المنشأة التابع لها الموظف';
COMMENT ON COLUMN org.employee_institutional_works.user_id IS 'معرف المستخدم الموظف';
COMMENT ON COLUMN org.employee_institutional_works.institutional_work_id IS 'معرف مجموعة العمل المؤسسية المخصصة';

-- 2. Add institutional_work_ids to catalog.products for structured PostgreSQL array overlap queries
ALTER TABLE catalog.products 
    ADD COLUMN IF NOT EXISTS institutional_work_ids BIGINT[] NOT NULL DEFAULT '{}'::bigint[];

CREATE INDEX IF NOT EXISTS products_inst_work_ids_gin 
    ON catalog.products USING GIN (institutional_work_ids);

COMMENT ON COLUMN catalog.products.institutional_work_ids IS 'مصفوفة معرفات مجموعات العمل المؤسسية المسموح لها برؤية المنتج';

COMMIT;
