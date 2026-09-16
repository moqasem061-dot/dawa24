DROP INDEX IF EXISTS catalog.idx_products_name_ar_norm_btree;
DROP INDEX IF EXISTS catalog.idx_products_name_en_lower_btree;
DROP INDEX IF EXISTS catalog.idx_saving_products_norm_name;
DROP INDEX IF EXISTS catalog.idx_customer_mappings_norm_name;
ALTER TABLE smartorder.run_config DROP COLUMN IF EXISTS match_language;
ALTER TABLE smartorder.criteria_profiles DROP COLUMN IF EXISTS match_language;
