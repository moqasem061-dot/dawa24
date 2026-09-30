-- 230_catalog_revisions.down.sql

BEGIN;

DROP TRIGGER IF EXISTS trg_catalog_products_revision_bump ON catalog.products;
DROP FUNCTION IF EXISTS catalog.bump_master_catalog_revision();
DROP TABLE IF EXISTS catalog.catalog_revisions;

COMMIT;
