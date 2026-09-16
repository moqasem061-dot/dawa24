-- 128_bulk_import_integrity (down)
--
-- Restores the pre-128 shape. The catalogue move is not reversed: rows that
-- migrated to the master-catalogue organisation stay there, because moving them
-- back would reintroduce the tenant-scoping bug this migration exists to fix
-- and there is no record of which organisation a row came from beyond the
-- import sessions themselves.

BEGIN;

DROP INDEX IF EXISTS catalog.product_variants_org_sku_key;

DROP POLICY IF EXISTS tenant_import_batches_isolation ON ingest.import_batches;
CREATE POLICY tenant_import_batches_isolation ON ingest.import_batches
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id));

DROP POLICY IF EXISTS tenant_import_progress_isolation ON ingest.import_progress;
CREATE POLICY tenant_import_progress_isolation ON ingest.import_progress
    AS RESTRICTIVE
    USING (platform.tenant_visible(organization_id));

ALTER TABLE catalog.import_sessions
    DROP COLUMN IF EXISTS new_categories;

ALTER TABLE catalog.import_sessions
    DROP CONSTRAINT IF EXISTS import_sessions_status_check;
-- 'committing' rows are transient by construction; any that survive to a
-- rollback of this migration were abandoned, so they are recorded as failed.
UPDATE catalog.import_sessions SET status = 'failed' WHERE status = 'committing';
ALTER TABLE catalog.import_sessions
    ADD CONSTRAINT import_sessions_status_check
    CHECK (status IN ('draft','ready','committed','cancelled','failed'));

COMMIT;
