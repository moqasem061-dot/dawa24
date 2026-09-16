-- 149_rls_gap_closure
--
-- Two gaps in tenant isolation, closed together.
--
-- 1. Nine tables carry organization_id — which is this project's definition of
--    tenant-owned data — and had no row-level security at all. Four of them
--    hold money or identity records. Any code path that reached them outside
--    db.InTx saw every tenant's rows.
--
-- 2. Seventy-six tables had RLS ENABLEd but not FORCEd. FORCE is what makes a
--    policy apply to the table owner as well. Every table here is owned by the
--    postgres role, so without FORCE any connection as the owner reads across
--    tenants silently. The application connects as dawa24_app and was never
--    affected, but migrations, the CLI and any operator session were, and a
--    future ownership change would have removed isolation with no signal.
--
-- Nullable organization_id follows the convention already established in
-- migrations 006 and 013: a NULL organization means a platform-owned row and
-- stays visible, because those tables mix platform rows with tenant rows.

BEGIN;

-- ---------------------------------------------------------------------------
-- 1. Tables that had no row-level security
-- ---------------------------------------------------------------------------

-- organization_id NOT NULL: straight tenant scoping.
DO $$
DECLARE t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'billing.subscription_histories',
        'billing.subscription_users',
        'catalog.import_sessions',
        'platform_admin.employee_activities'
    ] LOOP
        EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation ON %s
               USING (platform.tenant_visible(organization_id))
               WITH CHECK (platform.tenant_visible(organization_id))', t);
    END LOOP;
END $$;

-- organization_id NULLABLE: a NULL row is platform-owned and stays visible.
DO $$
DECLARE t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'billing.wallet_deposits',
        'catalog.match_decisions',
        'inventory.father_user_temparte_warehouses',
        'platform.audit_log',
        'platform_admin.documents'
    ] LOOP
        EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation ON %s
               USING (organization_id IS NULL OR platform.tenant_visible(organization_id))
               WITH CHECK (organization_id IS NULL OR platform.tenant_visible(organization_id))', t);
    END LOOP;
END $$;

-- ---------------------------------------------------------------------------
-- 2. FORCE on every table that already had RLS enabled
-- ---------------------------------------------------------------------------
-- Driven off the catalogue rather than a hand-written list, so a table added
-- between this migration being written and being applied is still covered.
DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN
        SELECT n.nspname AS sch, c.relname AS tbl
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relkind = 'r'
          AND c.relrowsecurity
          AND NOT c.relforcerowsecurity
          AND n.nspname NOT IN ('public', 'pg_catalog', 'information_schema')
    LOOP
        EXECUTE format('ALTER TABLE %I.%I FORCE ROW LEVEL SECURITY', r.sch, r.tbl);
    END LOOP;
END $$;

COMMIT;
