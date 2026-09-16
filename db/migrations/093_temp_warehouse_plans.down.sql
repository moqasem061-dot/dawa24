-- 093_temp_warehouse_plans.down.sql
BEGIN;

ALTER TABLE inventory.temp_warehouses
    DROP COLUMN IF EXISTS father_id,
    DROP COLUMN IF EXISTS file_path,
    DROP COLUMN IF EXISTS row_count,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS expires_at;

DROP TABLE IF EXISTS inventory.user_plan_temparte_warehouses;
DROP TABLE IF EXISTS inventory.plan_temparte_warehouses;
DROP TABLE IF EXISTS inventory.father_user_temparte_warehouses;

COMMIT;
