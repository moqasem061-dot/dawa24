-- 254_saving_guc_policies.up.sql
--
-- catalog.saving_products (083) and org.employee_institutional_works (084) were
-- written with policies that read app.current_tenant and app.is_system = 'true'.
-- The application sets app.current_org_id (platform.current_org_id()) and
-- app.is_system = 'on' (database.AsSystem), so a tenant could never see its
-- own rows: the policy failed closed on every read and write.
--
-- These policies now use platform.tenant_visible(organization_id), the same rule
-- every other tenant table uses. Owners see their own rows; other tenants see
-- none; AsSystem code paths see all rows, as the design intends.
-- No data changes. Policy USING clauses also apply to INSERT/UPDATE when no
-- WITH CHECK is given, so writes are restricted to the caller's organization.

BEGIN;

DROP POLICY IF EXISTS saving_products_tenant_isolation ON catalog.saving_products;
CREATE POLICY saving_products_tenant_isolation ON catalog.saving_products
    USING (platform.tenant_visible(organization_id));

DROP POLICY IF EXISTS emp_inst_works_tenant_isolation ON org.employee_institutional_works;
CREATE POLICY emp_inst_works_tenant_isolation ON org.employee_institutional_works
    USING (platform.tenant_visible(organization_id));

COMMIT;
