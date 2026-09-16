BEGIN;

DROP TABLE IF EXISTS platform_admin.cities CASCADE;
DROP TABLE IF EXISTS platform_admin.countries CASCADE;
DROP TABLE IF EXISTS platform_admin.system_settings CASCADE;

DROP SCHEMA IF EXISTS platform_admin CASCADE;

COMMIT;
