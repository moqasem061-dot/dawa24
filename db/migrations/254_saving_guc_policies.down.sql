-- 254_saving_guc_policies.down.sql
--
-- Restores the original policies from migrations 083 and 084. Note that these
-- read the wrong GUCs, so the rollback brings back the fail-closed behaviour.

BEGIN;

DROP POLICY IF EXISTS saving_products_tenant_isolation ON catalog.saving_products;
CREATE POLICY saving_products_tenant_isolation ON catalog.saving_products
    USING (
        organization_id = NULLIF(current_setting('app.current_tenant', true), '')::BIGINT
        OR current_setting('app.is_system', true) = 'true'
    );

DROP POLICY IF EXISTS emp_inst_works_tenant_isolation ON org.employee_institutional_works;
CREATE POLICY emp_inst_works_tenant_isolation ON org.employee_institutional_works
    USING (
        organization_id = NULLIF(current_setting('app.current_tenant', true), '')::BIGINT
        OR current_setting('app.is_system', true) = 'true'
    );

COMMIT;
