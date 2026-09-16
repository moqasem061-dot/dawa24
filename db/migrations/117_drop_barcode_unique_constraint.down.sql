-- 117_drop_barcode_unique_constraint (down)

BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS products_org_barcode_uniq
    ON catalog.products (organization_id, lower(btrim(barcode)))
    WHERE deleted_at IS NULL AND btrim(barcode) <> '';

COMMIT;
