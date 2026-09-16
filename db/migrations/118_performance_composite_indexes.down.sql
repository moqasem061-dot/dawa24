-- 118_performance_composite_indexes (down)

BEGIN;

DROP INDEX IF EXISTS catalog.idx_products_org_status;
DROP INDEX IF EXISTS catalog.idx_products_category_brand;
DROP INDEX IF EXISTS catalog.idx_products_dosage;
DROP INDEX IF EXISTS catalog.idx_products_price_sort;
DROP INDEX IF EXISTS catalog.idx_products_name_en_trgm;
DROP INDEX IF EXISTS catalog.idx_products_scientific_trgm;
DROP INDEX IF EXISTS catalog.idx_products_active_trgm;
DROP INDEX IF EXISTS catalog.idx_saving_products_org_prod;
DROP INDEX IF EXISTS catalog.idx_saving_products_user;
DROP INDEX IF EXISTS commerce.idx_orders_customer_status;
DROP INDEX IF EXISTS commerce.idx_orders_org_status;
DROP INDEX IF EXISTS commerce.idx_carts_org_user;
DROP INDEX IF EXISTS compare.idx_compare_file_rows_file_id;
DROP INDEX IF EXISTS compare.idx_compare_file_rows_norm_name;
DROP INDEX IF EXISTS compare.idx_compare_files_user_status;
DROP INDEX IF EXISTS compare.idx_compare_sessions_user_active;
DROP INDEX IF EXISTS notifications.idx_notif_logs_user_status;

COMMIT;
