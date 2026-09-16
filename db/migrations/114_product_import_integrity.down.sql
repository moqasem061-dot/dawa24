-- 114_product_import_integrity (down)

BEGIN;

DROP INDEX IF EXISTS catalog.products_org_barcode_uniq;
DROP INDEX IF EXISTS catalog.products_org_sku_uniq;
DROP INDEX IF EXISTS catalog.products_org_barcode_lookup;
DROP INDEX IF EXISTS catalog.products_org_sku_lookup;

COMMIT;
