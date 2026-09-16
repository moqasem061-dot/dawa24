-- 117_drop_barcode_unique_constraint (up)
--
-- Remove unique constraint on barcode for products to allow duplicate/shared
-- barcodes across master catalog items and package variations.

BEGIN;

DROP INDEX IF EXISTS catalog.products_org_barcode_uniq;

COMMIT;
