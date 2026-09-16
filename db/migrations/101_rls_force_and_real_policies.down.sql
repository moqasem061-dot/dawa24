-- 101_rls_force_and_real_policies (down)
--
-- Restores the permissive policies and drops FORCE. This reinstates the holes
-- 101 closed, which is the only reason to run it.

BEGIN;

ALTER TABLE org.delivery_bands              NO FORCE ROW LEVEL SECURITY;
ALTER TABLE org.roles                       NO FORCE ROW LEVEL SECURITY;
ALTER TABLE org.employee_institutional_works NO FORCE ROW LEVEL SECURITY;
ALTER TABLE catalog.saving_products         NO FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS delivery_bands_tenant_isolation ON org.delivery_bands;
CREATE POLICY delivery_bands_tenant_isolation ON org.delivery_bands USING (true);

DROP POLICY IF EXISTS roles_tenant_isolation ON org.roles;
CREATE POLICY roles_tenant_isolation ON org.roles USING (true);

COMMIT;
