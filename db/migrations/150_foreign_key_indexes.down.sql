-- Reverses 150_foreign_key_indexes.
--
-- Drops the thirty-five indexes added for unindexed foreign keys. The
-- foreign key constraints themselves are untouched; only their supporting
-- indexes go, which restores the pre-migration state exactly.

BEGIN;

DROP INDEX IF EXISTS billing.wallet_deposits_reviewed_by_idx;
DROP INDEX IF EXISTS billing.wallet_deposits_transaction_id_idx;
DROP INDEX IF EXISTS catalog.import_sessions_created_by_idx;
DROP INDEX IF EXISTS catalog.match_decisions_chosen_product_idx;
DROP INDEX IF EXISTS hr.job_applications_assigned_role_idx;
DROP INDEX IF EXISTS hr.job_applications_branch_id_idx;
DROP INDEX IF EXISTS identity.identity_roles_created_by_idx;
DROP INDEX IF EXISTS ingest.catalog_import_rows_variant_idx;
DROP INDEX IF EXISTS ingest.catalog_import_rows_org_idx;
DROP INDEX IF EXISTS ingest.catalog_import_rows_product_idx;
DROP INDEX IF EXISTS ingest.catalog_imports_created_by_idx;
DROP INDEX IF EXISTS org.org_role_permissions_permission_idx;
DROP INDEX IF EXISTS org.org_roles_created_by_idx;
DROP INDEX IF EXISTS promo.ad_impressions_user_idx;
DROP INDEX IF EXISTS promo.ads_ad_plan_idx;
DROP INDEX IF EXISTS promo.ads_reviewed_by_idx;
DROP INDEX IF EXISTS promo.offer_sponsorships_request_idx;
DROP INDEX IF EXISTS promo.offer_sponsorships_reviewed_by_idx;
DROP INDEX IF EXISTS promo.sponsorship_purchases_approved_idx;
DROP INDEX IF EXISTS promo.sponsorship_purchases_package_idx;
DROP INDEX IF EXISTS promo.sponsorship_purchases_payment_idx;
DROP INDEX IF EXISTS promo.sponsorship_requests_package_idx;
DROP INDEX IF EXISTS promo.sponsorship_requests_purchase_idx;
DROP INDEX IF EXISTS promo.sponsorship_requests_reviewed_idx;
DROP INDEX IF EXISTS smartorder.column_mappings_org_idx;
DROP INDEX IF EXISTS smartorder.criteria_profiles_last_branch_idx;
DROP INDEX IF EXISTS smartorder.line_candidates_branch_idx;
DROP INDEX IF EXISTS smartorder.line_candidates_variant_idx;
DROP INDEX IF EXISTS smartorder.line_selections_skipped_cand_idx;
DROP INDEX IF EXISTS smartorder.run_config_org_idx;
DROP INDEX IF EXISTS smartorder.run_events_org_idx;
DROP INDEX IF EXISTS smartorder.run_lines_matched_product_idx;
DROP INDEX IF EXISTS smartorder.run_lines_consolidated_into_idx;
DROP INDEX IF EXISTS smartorder.runs_order_idx;
DROP INDEX IF EXISTS smartorder.runs_branch_idx;

COMMIT;
