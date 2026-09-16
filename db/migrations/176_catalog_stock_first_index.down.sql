-- Migration 176 Down: drop the catalogue availability indexes.
BEGIN;
DROP INDEX IF EXISTS inventory.idx_stocks_variant_available;
DROP INDEX IF EXISTS catalog.idx_product_variants_product_live;
COMMIT;
