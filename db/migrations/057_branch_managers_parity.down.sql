-- Migration 057 Down

BEGIN;

DROP INDEX IF EXISTS org.idx_branches_type;
DROP INDEX IF EXISTS org.idx_branches_manager;

ALTER TABLE org.branches
    DROP COLUMN IF EXISTS manager_id,
    DROP COLUMN IF EXISTS code,
    DROP COLUMN IF EXISTS warehouse_type,
    DROP COLUMN IF EXISTS has_cold_storage,
    DROP COLUMN IF EXISTS capacity_sqm,
    DROP COLUMN IF EXISTS operating_hours;

COMMIT;
