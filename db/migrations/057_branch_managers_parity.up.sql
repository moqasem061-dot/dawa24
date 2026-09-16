-- Migration 057: Branch Managers & Location Metadata Parity
-- Associates branches with designated managers and extends physical branch metadata.

BEGIN;

-- 1. Extend org.branches with manager and facility attributes
ALTER TABLE org.branches
    ADD COLUMN IF NOT EXISTS manager_id       BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS code             TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS warehouse_type   TEXT NOT NULL DEFAULT 'warehouse'
        CHECK (warehouse_type IN ('warehouse','fast_hub','pharmacy_branch','cold_depot')),
    ADD COLUMN IF NOT EXISTS has_cold_storage BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS capacity_sqm     NUMERIC(10,2) NOT NULL DEFAULT 0.00,
    ADD COLUMN IF NOT EXISTS operating_hours  TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_branches_manager ON org.branches (manager_id);
CREATE INDEX IF NOT EXISTS idx_branches_type    ON org.branches (warehouse_type);

COMMIT;
