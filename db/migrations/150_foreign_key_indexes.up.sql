-- 150_foreign_key_indexes
--
-- Thirty-five foreign keys had no supporting index.
--
-- Postgres does not create one for a foreign key, only for a primary key or a
-- unique constraint. Without it, every join through the key and — more
-- expensively — every DELETE or UPDATE on the *referenced* table has to scan
-- the referencing table in full to check the constraint. On
-- ingest.catalog_import_rows that is 77,054 rows scanned per parent row
-- touched, three times over.
--
-- The concentration is not random: promo (sponsorships and ads) and smartorder
-- are the two least finished modules, and they account for twenty-two of the
-- thirty-five.
--
-- CREATE INDEX is not CONCURRENTLY here because the migration runner wraps each
-- migration in a transaction and CONCURRENTLY cannot run inside one. These
-- tables are small enough that the lock is measured in milliseconds; the two
-- large ones (catalog_import_rows, run_lines) are import scratch tables that no
-- interactive request reads.

BEGIN;

-- billing
CREATE INDEX IF NOT EXISTS wallet_deposits_reviewed_by_idx     ON billing.wallet_deposits (reviewed_by);
CREATE INDEX IF NOT EXISTS wallet_deposits_transaction_id_idx  ON billing.wallet_deposits (transaction_id);

-- catalog
CREATE INDEX IF NOT EXISTS import_sessions_created_by_idx      ON catalog.import_sessions (created_by);
CREATE INDEX IF NOT EXISTS match_decisions_chosen_product_idx  ON catalog.match_decisions (chosen_product_id);

-- hr
CREATE INDEX IF NOT EXISTS job_applications_assigned_role_idx  ON hr.job_applications (assigned_role_key);
CREATE INDEX IF NOT EXISTS job_applications_branch_id_idx      ON hr.job_applications (branch_id);

-- identity
CREATE INDEX IF NOT EXISTS identity_roles_created_by_idx       ON identity.roles (created_by);

-- ingest
CREATE INDEX IF NOT EXISTS catalog_import_rows_variant_idx     ON ingest.catalog_import_rows (variant_id);
CREATE INDEX IF NOT EXISTS catalog_import_rows_org_idx         ON ingest.catalog_import_rows (organization_id);
CREATE INDEX IF NOT EXISTS catalog_import_rows_product_idx     ON ingest.catalog_import_rows (product_id);
CREATE INDEX IF NOT EXISTS catalog_imports_created_by_idx      ON ingest.catalog_imports (created_by);

-- org
CREATE INDEX IF NOT EXISTS org_role_permissions_permission_idx ON org.role_permissions (permission_key);
CREATE INDEX IF NOT EXISTS org_roles_created_by_idx            ON org.roles (created_by);

-- promo
CREATE INDEX IF NOT EXISTS ad_impressions_user_idx             ON promo.ad_impressions (user_id);
CREATE INDEX IF NOT EXISTS ads_ad_plan_idx                     ON promo.ads (ad_plan_id);
CREATE INDEX IF NOT EXISTS ads_reviewed_by_idx                 ON promo.ads (reviewed_by);
CREATE INDEX IF NOT EXISTS offer_sponsorships_request_idx      ON promo.offer_sponsorships (sponsorship_request_id);
CREATE INDEX IF NOT EXISTS offer_sponsorships_reviewed_by_idx  ON promo.offer_sponsorships (reviewed_by);
CREATE INDEX IF NOT EXISTS sponsorship_purchases_approved_idx  ON promo.sponsorship_purchases (approved_by);
CREATE INDEX IF NOT EXISTS sponsorship_purchases_package_idx   ON promo.sponsorship_purchases (package_id);
CREATE INDEX IF NOT EXISTS sponsorship_purchases_payment_idx   ON promo.sponsorship_purchases (payment_id);
CREATE INDEX IF NOT EXISTS sponsorship_requests_package_idx    ON promo.sponsorship_requests (package_id);
CREATE INDEX IF NOT EXISTS sponsorship_requests_purchase_idx   ON promo.sponsorship_requests (purchase_id);
CREATE INDEX IF NOT EXISTS sponsorship_requests_reviewed_idx   ON promo.sponsorship_requests (reviewed_by);

-- smartorder
CREATE INDEX IF NOT EXISTS column_mappings_org_idx             ON smartorder.column_mappings (organization_id);
CREATE INDEX IF NOT EXISTS criteria_profiles_last_branch_idx   ON smartorder.criteria_profiles (last_branch_id);
CREATE INDEX IF NOT EXISTS line_candidates_branch_idx          ON smartorder.line_candidates (branch_id);
CREATE INDEX IF NOT EXISTS line_candidates_variant_idx         ON smartorder.line_candidates (variant_id);
CREATE INDEX IF NOT EXISTS line_selections_skipped_cand_idx    ON smartorder.line_selections (skipped_candidate_id);
CREATE INDEX IF NOT EXISTS run_config_org_idx                  ON smartorder.run_config (organization_id);
CREATE INDEX IF NOT EXISTS run_events_org_idx                  ON smartorder.run_events (organization_id);
CREATE INDEX IF NOT EXISTS run_lines_matched_product_idx       ON smartorder.run_lines (matched_product_id);
CREATE INDEX IF NOT EXISTS run_lines_consolidated_into_idx     ON smartorder.run_lines (consolidated_into_line_id);
CREATE INDEX IF NOT EXISTS runs_order_idx                      ON smartorder.runs (order_id);
CREATE INDEX IF NOT EXISTS runs_branch_idx                     ON smartorder.runs (branch_id);

COMMIT;
