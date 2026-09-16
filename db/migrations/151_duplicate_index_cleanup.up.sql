-- 151_duplicate_index_cleanup
--
-- Nine indexes that duplicate another index on the same table, over the same
-- columns, with the same predicate. Each pair was created by two different
-- migrations that both decided the column needed an index; nothing detects
-- that, so the duplicate is maintained on every write forever.
--
-- In every case the survivor is the one that also enforces something — a
-- UNIQUE constraint or a unique index — or, where neither is unique, the one
-- whose name says what it is for. Nothing here changes which queries can be
-- planned; a duplicate index is never the only way to answer a query.
--
-- Indexes that are merely *unused* are deliberately not touched. Thirty of them
-- have never been scanned since the database was created, which is suggestive
-- but not conclusive: features that have not been exercised yet cannot have
-- used their indexes. pg_stat_statements cannot be installed here either --
-- shared_preload_libraries is empty on this instance and only a server restart
-- can change that. Dropping them belongs to a later, measured pass.

BEGIN;

-- Same columns (user_id, status); the survivor's name says so.
DROP INDEX IF EXISTS billing.idx_wallet_deposits_user;

-- Two GIN indexes on institutional_work_ids.
DROP INDEX IF EXISTS catalog.product_index_institutional_idx;

-- Shadows the unique index products_org_sku_uniq exactly, predicate included.
DROP INDEX IF EXISTS catalog.products_org_sku_lookup;

-- Shadows the carts_user_unique constraint.
DROP INDEX IF EXISTS commerce.carts_user_idx;

-- Same columns and predicate; keeps the idx_-prefixed name used elsewhere in
-- the compare schema.
DROP INDEX IF EXISTS compare.compare_files_user_status_idx;

-- Three indexes on job_seeker_profiles(user_id); uq_job_seeker_user enforces
-- the constraint, so both plain copies go.
DROP INDEX IF EXISTS hr.idx_hr_job_seeker_profiles_user_id;
DROP INDEX IF EXISTS hr.idx_job_seeker_profiles_user;

-- Shadows the uq_import_progress_session constraint.
DROP INDEX IF EXISTS ingest.idx_ingest_import_progress_session_id;

-- Shadows the translations_key_key unique constraint.
DROP INDEX IF EXISTS platform.idx_translations_key;

COMMIT;
