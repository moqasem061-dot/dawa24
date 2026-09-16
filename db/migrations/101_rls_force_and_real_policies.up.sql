-- 101_rls_force_and_real_policies (up)
--
-- Two row-level-security holes found by inspecting the live database, not the
-- migrations.
--
-- 1. ENABLE WITHOUT FORCE. Four tenant-owned tables had RLS enabled but not
--    forced. Postgres exempts a table's owner from its own policies unless
--    FORCE is set, and this deployment connects as the table owner, so RLS on
--    those tables was inert. AGENTS.md rule R8 requires ENABLE + FORCE; these
--    four had only half of it.
--
-- 2. POLICIES THAT PERMIT EVERYTHING. org.delivery_bands and org.roles carried
--    policies whose USING clause is literally `true`. RLS was "on" and isolated
--    nothing. Both tables have an organization_id and are tenant-owned.
--
-- Both tables are empty in the live database (0 rows each), so replacing the
-- policies cannot hide existing data from anyone.
--
-- Verified 2026-08-20 against the live schema.

BEGIN;

-- 1. Real tenant policies where the old ones said `true`.
DROP POLICY IF EXISTS delivery_bands_tenant_isolation ON org.delivery_bands;
CREATE POLICY delivery_bands_tenant_isolation ON org.delivery_bands
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));

DROP POLICY IF EXISTS roles_tenant_isolation ON org.roles;
CREATE POLICY roles_tenant_isolation ON org.roles
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));

-- 2. FORCE, so the owner is subject to the policy too.
ALTER TABLE org.delivery_bands              FORCE ROW LEVEL SECURITY;
ALTER TABLE org.roles                       FORCE ROW LEVEL SECURITY;
ALTER TABLE org.employee_institutional_works FORCE ROW LEVEL SECURITY;
ALTER TABLE catalog.saving_products         FORCE ROW LEVEL SECURITY;

COMMIT;
