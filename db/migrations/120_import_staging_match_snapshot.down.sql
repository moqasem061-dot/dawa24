-- 120_import_staging_match_snapshot (down)

BEGIN;

ALTER TABLE catalog.import_staging_rows
    ADD CONSTRAINT import_staging_rows_matched_product_id_fkey
    FOREIGN KEY (matched_product_id) REFERENCES catalog.products(id) ON DELETE SET NULL;

COMMIT;
