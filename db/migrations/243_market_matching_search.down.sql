BEGIN;
DROP INDEX IF EXISTS compare.compare_file_rows_market_name_trgm;
DROP INDEX IF EXISTS compare.compare_file_rows_matched_product;
DROP INDEX IF EXISTS catalog.products_market_names_trgm;
COMMIT;
