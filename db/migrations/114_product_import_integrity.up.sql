-- 114_product_import_integrity (up)
--
-- Bulk import matched nothing and enforced nothing. catalog.products carried no
-- unique constraint on sku or barcode, so the "upsert" could only ever insert:
-- re-uploading a corrected supplier price list duplicated the whole catalogue,
-- and the update count the admin screen reported was a constant zero.
--
-- Two things are needed. Lookups by (organization_id, sku) and
-- (organization_id, barcode) must be fast, because matching ten thousand
-- imported rows against an existing catalogue is the hot path of every import.
-- And a second row claiming an identifier that is already taken within the same
-- organisation must be refused by the database, not merely avoided by the
-- importer -- two concurrent imports of the same file would otherwise both see
-- an empty catalogue and both insert.

BEGIN;

-- Identifiers are compared folded, so the indexes must be built on the same
-- expression the import matches with: lower(btrim(...)). A partial index keeps
-- soft-deleted rows and rows with no identifier out, both of which are common
-- and neither of which should collide.
CREATE INDEX IF NOT EXISTS products_org_sku_lookup
    ON catalog.products (organization_id, lower(btrim(sku)))
    WHERE deleted_at IS NULL AND btrim(sku) <> '';

CREATE INDEX IF NOT EXISTS products_org_barcode_lookup
    ON catalog.products (organization_id, lower(btrim(barcode)))
    WHERE deleted_at IS NULL AND btrim(barcode) <> '';

-- The unique constraints are applied only where the existing data already
-- satisfies them.
--
-- A migration must not fail on a deployment whose catalogue accumulated
-- duplicates under the old importer, and it must not silently delete or rename
-- a pharmacy's rows to force the constraint through. So it checks first, and
-- where duplicates exist it leaves a notice in the migration log for an
-- operator to reconcile deliberately. The lookup indexes above are created
-- either way, so import matching is fast even before the data is cleaned.
DO $$
DECLARE
    dup_sku     BIGINT;
    dup_barcode BIGINT;
BEGIN
    SELECT count(*) INTO dup_sku FROM (
        SELECT 1 FROM catalog.products
        WHERE deleted_at IS NULL AND btrim(sku) <> ''
        GROUP BY organization_id, lower(btrim(sku))
        HAVING count(*) > 1
    ) d;

    IF dup_sku = 0 THEN
        CREATE UNIQUE INDEX IF NOT EXISTS products_org_sku_uniq
            ON catalog.products (organization_id, lower(btrim(sku)))
            WHERE deleted_at IS NULL AND btrim(sku) <> '';
    ELSE
        RAISE NOTICE 'products_org_sku_uniq not created: % duplicate (organization_id, sku) groups exist and need reconciling first', dup_sku;
    END IF;

    SELECT count(*) INTO dup_barcode FROM (
        SELECT 1 FROM catalog.products
        WHERE deleted_at IS NULL AND btrim(barcode) <> ''
        GROUP BY organization_id, lower(btrim(barcode))
        HAVING count(*) > 1
    ) d;

    IF dup_barcode = 0 THEN
        CREATE UNIQUE INDEX IF NOT EXISTS products_org_barcode_uniq
            ON catalog.products (organization_id, lower(btrim(barcode)))
            WHERE deleted_at IS NULL AND btrim(barcode) <> '';
    ELSE
        RAISE NOTICE 'products_org_barcode_uniq not created: % duplicate (organization_id, barcode) groups exist and need reconciling first', dup_barcode;
    END IF;
END $$;

COMMENT ON INDEX catalog.products_org_sku_lookup IS
    'يخدم مطابقة الأصناف بكود الصنف أثناء الاستيراد المجمّع';
COMMENT ON INDEX catalog.products_org_barcode_lookup IS
    'يخدم مطابقة الأصناف بالباركود أثناء الاستيراد المجمّع';

COMMIT;
