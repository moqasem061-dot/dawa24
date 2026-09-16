-- 128_bulk_import_integrity (up)
--
-- Fixes surfaced by the bulk-import audit, applied as expand-only changes the
-- currently deployed image tolerates:
--
--   1. catalog.import_sessions gains the 'committing' status, the atomic claim a
--      commit takes before writing anything. Without it a crash between "the
--      catalogue was written" and "the session row said committed" left a ready
--      session that a second click would commit twice.
--   2. catalog.import_sessions gains new_categories, which prepare computed and
--      the review screen promised but nothing ever stored.
--   3. The master catalogue gets its own organisation. Resolution used to pick
--      the lowest-id approved organisation, so on this deployment the whole
--      imported catalogue lives inside a customer pharmacy's tenant. The
--      canonical organisation is created here and the import-created catalogue
--      is moved into it.
--   4. Variants get a real identity constraint: (organization_id, sku) among
--      live rows. Until now two concurrent imports — or two copies of one row
--      in a single file — could insert duplicates with nothing at the database
--      level stopping them.
--   5. ingest.import_batches / ingest.import_progress carried RESTRICTIVE-only
--      RLS policies. A restrictive policy can only narrow; with no permissive
--      policy alongside it, every role — including the table owner — is denied
--      every row forever. They get the standard permissive tenant policy.

BEGIN;

-- 1. The committing claim status.
ALTER TABLE catalog.import_sessions
    DROP CONSTRAINT IF EXISTS import_sessions_status_check;
ALTER TABLE catalog.import_sessions
    ADD CONSTRAINT import_sessions_status_check
    CHECK (status IN ('draft','committing','ready','committed','cancelled','failed'));

-- 2. Categories prepare found nothing existing covers.
ALTER TABLE catalog.import_sessions
    ADD COLUMN IF NOT EXISTS new_categories JSONB NOT NULL DEFAULT '[]'::JSONB;
COMMENT ON COLUMN catalog.import_sessions.new_categories IS
    'التصنيفات الجديدة التي سيضيفها الاستيراد بعد مراجعة المشرف';

-- 3. The canonical master-catalogue organisation, then the move.
INSERT INTO org.organizations (name, type, status)
SELECT '{"ar":"دواء 24 - الكتالوج المعتمد","en":"Dawa24 Master Catalog"}'::jsonb,
       'company', 'approved'
WHERE NOT EXISTS (
    SELECT 1 FROM org.organizations
    WHERE deleted_at IS NULL AND name->>'en' = 'Dawa24 Master Catalog'
);

-- Every organisation that holds catalogue-import state but is not the master
-- org got that state through the old lowest-id resolution. Its imported rows
-- belong to the master catalogue.
CREATE TEMP TABLE _drained_orgs ON COMMIT DROP AS
SELECT DISTINCT s.organization_id AS id
FROM catalog.import_sessions s
CROSS JOIN org.organizations m
WHERE m.deleted_at IS NULL AND m.name->>'en' = 'Dawa24 Master Catalog'
  AND s.organization_id <> m.id;

CREATE TEMP TABLE _moved_products ON COMMIT DROP AS
SELECT p.id
FROM catalog.products p
JOIN _drained_orgs d ON d.id = p.organization_id;

UPDATE catalog.products p
SET organization_id = m.id, updated_at = now()
FROM org.organizations m, _drained_orgs d
WHERE m.deleted_at IS NULL AND m.name->>'en' = 'Dawa24 Master Catalog'
  AND p.organization_id = d.id;

UPDATE catalog.product_index pi
SET organization_id = m.id
FROM org.organizations m, _moved_products mp
WHERE m.deleted_at IS NULL AND m.name->>'en' = 'Dawa24 Master Catalog'
  AND pi.product_id = mp.id;

-- The sessions follow their rows, so a still-open review commits into the same
-- organisation its staged matches point at.
UPDATE catalog.import_sessions s
SET organization_id = m.id
FROM org.organizations m, _drained_orgs d
WHERE m.deleted_at IS NULL AND m.name->>'en' = 'Dawa24 Master Catalog'
  AND s.organization_id = d.id;

-- 4. Variant identity. Partial: soft-deleted rows and blank SKUs stay out of
--    the constraint, exactly matching how the importer reads them.
CREATE UNIQUE INDEX IF NOT EXISTS product_variants_org_sku_key
    ON catalog.product_variants (organization_id, sku)
    WHERE deleted_at IS NULL AND sku <> '';

-- 5. Usable RLS on the dormant batch/progress tables.
DROP POLICY IF EXISTS tenant_import_batches_isolation ON ingest.import_batches;
CREATE POLICY tenant_import_batches_isolation ON ingest.import_batches
    AS PERMISSIVE FOR ALL
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));

DROP POLICY IF EXISTS tenant_import_progress_isolation ON ingest.import_progress;
CREATE POLICY tenant_import_progress_isolation ON ingest.import_progress
    AS PERMISSIVE FOR ALL
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));

COMMIT;
