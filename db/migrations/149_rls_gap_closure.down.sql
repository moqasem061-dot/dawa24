-- Reverses 149_rls_gap_closure.
--
-- FORCE is dropped from every table it was added to; the nine policies created
-- here are dropped along with the RLS flag on their tables. Tables that had RLS
-- before this migration keep it — only FORCE is removed, which restores the
-- pre-migration state exactly.

BEGIN;

DO $$
DECLARE t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'billing.subscription_histories',
        'billing.subscription_users',
        'catalog.import_sessions',
        'platform_admin.employee_activities',
        'billing.wallet_deposits',
        'catalog.match_decisions',
        'inventory.father_user_temparte_warehouses',
        'platform.audit_log',
        'platform_admin.documents'
    ] LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %s', t);
        EXECUTE format('ALTER TABLE %s NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %s DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;

DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN
        SELECT n.nspname AS sch, c.relname AS tbl
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relkind = 'r'
          AND c.relforcerowsecurity
          AND n.nspname NOT IN ('public', 'pg_catalog', 'information_schema')
    LOOP
        EXECUTE format('ALTER TABLE %I.%I NO FORCE ROW LEVEL SECURITY', r.sch, r.tbl);
    END LOOP;
END $$;

COMMIT;
