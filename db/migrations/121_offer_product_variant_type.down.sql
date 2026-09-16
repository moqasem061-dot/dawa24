-- Migration 121: Down
BEGIN;

DROP INDEX IF EXISTS catalog.product_variants_offer_id_idx;
DROP INDEX IF EXISTS catalog.product_variants_variant_type_idx;

ALTER TABLE catalog.product_variants DROP COLUMN IF EXISTS offer_id;
ALTER TABLE catalog.product_variants DROP COLUMN IF EXISTS variant_type;

COMMIT;
