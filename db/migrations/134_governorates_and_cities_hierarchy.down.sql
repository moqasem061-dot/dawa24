-- Migration 134 Down: Drop Governorates & remove governorate_id from cities
BEGIN;

ALTER TABLE platform_admin.cities DROP COLUMN IF EXISTS governorate_id;
DROP TABLE IF EXISTS platform_admin.governorates CASCADE;

COMMIT;
