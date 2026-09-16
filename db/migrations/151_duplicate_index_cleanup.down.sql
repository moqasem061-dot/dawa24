-- Reverses 151_duplicate_index_cleanup by recreating the nine duplicate
-- indexes exactly as they were defined.

BEGIN;

CREATE INDEX IF NOT EXISTS idx_wallet_deposits_user
    ON billing.wallet_deposits USING btree (user_id, status);

CREATE INDEX IF NOT EXISTS product_index_institutional_idx
    ON catalog.product_index USING gin (institutional_work_ids);

CREATE INDEX IF NOT EXISTS products_org_sku_lookup
    ON catalog.products USING btree (organization_id, lower(btrim(sku)))
    WHERE deleted_at IS NULL AND btrim(sku) <> '';

CREATE INDEX IF NOT EXISTS carts_user_idx
    ON commerce.carts USING btree (user_id);

CREATE INDEX IF NOT EXISTS compare_files_user_status_idx
    ON compare.files USING btree (user_id, status)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_hr_job_seeker_profiles_user_id
    ON hr.job_seeker_profiles USING btree (user_id);

CREATE INDEX IF NOT EXISTS idx_job_seeker_profiles_user
    ON hr.job_seeker_profiles USING btree (user_id);

CREATE INDEX IF NOT EXISTS idx_ingest_import_progress_session_id
    ON ingest.import_progress USING btree (session_id);

CREATE INDEX IF NOT EXISTS idx_translations_key
    ON platform.translations USING btree (key);

COMMIT;
