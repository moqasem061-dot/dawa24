-- 046_platform_overhaul (down)
BEGIN;

DROP TABLE IF EXISTS org.custom_roles CASCADE;

ALTER TABLE hr.job_applications DROP COLUMN IF EXISTS applicant_user_id;

ALTER TABLE catalog.product_variants DROP COLUMN IF EXISTS batch_number;
ALTER TABLE catalog.product_variants DROP COLUMN IF EXISTS expiry_date;
ALTER TABLE catalog.product_variants DROP COLUMN IF EXISTS min_order_qty;
ALTER TABLE catalog.product_variants DROP COLUMN IF EXISTS branch_id;

ALTER TABLE org.branches DROP COLUMN IF EXISTS code;
ALTER TABLE org.branches DROP COLUMN IF EXISTS google_maps_url;
ALTER TABLE org.branches DROP COLUMN IF EXISTS manager_name;
ALTER TABLE org.branches DROP COLUMN IF EXISTS warehouse_type;
ALTER TABLE org.branches DROP COLUMN IF EXISTS has_cold_storage;
ALTER TABLE org.branches DROP COLUMN IF EXISTS capacity_sqm;
ALTER TABLE org.branches DROP COLUMN IF EXISTS operating_hours;

COMMIT;
