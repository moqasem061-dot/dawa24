-- =============================================================================
-- Dawa24 - Production Database Cleanup & Sanitization Script
-- =============================================================================
-- PURPOSE:
--   Prepares a staging/testing database for clean production launch.
--   - Removes all testing organizations, branches, members, and non-admin users.
--   - Purges all test commerce data (orders, shipments, carts, quotas).
--   - Purges all test inventory stocks, movements, and warehouses.
--   - Purges all test offers, ads, promotions, and sponsorships.
--   - Purges all logs, error traces, import sessions, staging tables, and notifications.
--   - Purges test chat conversations, assistant memories, and bot tokens.
--   - Purges catalog decision memory and search caches.
--
-- PRESERVED DATA (CRITICAL):
--   ✓ Master Catalog: Categories, Brands, Products, Variants, Product Infos.
--   ✓ Subscription System: Plans, Plan Features, Plan Types, Payment Methods.
--   ✓ Platform Settings: System Settings, Policies, SEO, Managed Pages, CMS Blocks.
--   ✓ Geography: Countries, Governorates, Cities, Currencies, Languages.
--   ✓ Security & RBAC: System Permissions, Roles, and the Super Admin user.
--   ✓ Templates: Email and notification message templates.
-- =============================================================================
-- If a previous query failed, reset the session transaction state first:
ROLLBACK;

BEGIN;

DO $$
DECLARE
    tbl text;
    tables_to_truncate text[] := ARRAY[
        -- 1. Chat, Assistant & Bot Bridges
        'assistant.tool_audit',
        'assistant.attachments',
        'assistant.messages',
        'assistant.turns',
        'assistant.organization_memories',
        'assistant.conversations',
        'chat.messages',
        'chat.participants',
        'chat.conversations',
        'telegram.deliveries',
        'telegram.link_tokens',
        'telegram.links',
        'telegram.processed_updates',
        'whatsapp.deliveries',
        'whatsapp.link_tokens',
        'whatsapp.links',
        'whatsapp.processed_messages',

        -- 2. SmartOrder & Temporary Decision/Finder Data
        'smartorder.line_selections',
        'smartorder.line_candidates',
        'smartorder.run_candidates',
        'smartorder.run_lines',
        'smartorder.run_events',
        'smartorder.run_files',
        'smartorder.run_config',
        'smartorder.runs',
        'catalog.decision_memory_preferences',
        'catalog.finder_results',

        -- 3. Ingest, Staging & Import Batch Logs
        'ingest.catalog_import_rows',
        'ingest.catalog_imports',
        'ingest.import_batches',
        'ingest.import_progress',
        'ingest.import_rows',
        'ingest.import_sessions',
        'ingest.file_uploads',
        'catalog.import_staging_rows',
        'catalog.import_sessions',
        'platform.import_run_rows',
        'platform.import_runs',
        'compare.file_rows',
        'compare.files',

        -- 4. Audit, System, Error & Visit Logs
        'platform.audit_log',
        'platform_admin.error_logs',
        'platform_admin.sql_logs',
        'platform_admin.visitors',
        'platform_admin.employee_activities',
        'platform_admin.contact_messages',
        'platform_admin.document_requests',
        'ai.usage_events',
        'notifications.admin_notifications',
        'notifications.logs',

        -- 5. Commerce & Orders (All Test Transactions)
        'commerce.order_lines',
        'commerce.order_status_history',
        'commerce.order_shipments',
        'commerce.orders',
        'commerce.cart_items',
        'commerce.carts',
        'commerce.variant_branch_quota_releases',
        'commerce.quote_requests',

        -- 6. Inventory & Warehouses (Test Stocks & Movements)
        'inventory.stock_movements',
        'inventory.warehouse_transfers',
        'inventory.stocks',
        'inventory.temp_warehouses',
        'inventory.father_user_temparte_warehouses',
        'inventory.plan_temparte_warehouses',
        'inventory.user_plan_temparte_warehouses',
        'inventory.supplier_trackings',
        'inventory.warehouses',

        -- 7. Promotions, Ads & Sponsorships
        'promo.ad_clicks',
        'promo.ad_impressions',
        'promo.ads',
        'promo.offer_clicks',
        'promo.offer_views',
        'promo.offer_promotions',
        'promo.offer_location_covers',
        'promo.offer_products',
        'promo.special_offer_locations',
        'promo.special_offer_products',
        'promo.special_offers',
        'promo.sponsorship_credit_entries',
        'promo.sponsorship_purchases',
        'promo.sponsorship_requests',
        'promo.offer_sponsorships',
        'promo.offer_package_features',
        'promo.offer_packages',
        'promo.offers',
        'promo.highlight_section_items',
        'promo.highlight_sections',

        -- 8. Billing Transactions & User Subscriptions (Plans are KEPT)
        'billing.wallet_transactions',
        'billing.wallet_deposits',
        'billing.wallet_withdrawals',
        'billing.wallets',
        'billing.payment_histories',
        'billing.payments',
        'billing.subscription_histories',
        'billing.subscription_users',
        'billing.user_plan_histories',
        'billing.subscriptions',
        'compare.subscriptions',
        'compare.subscription_users',
        'compare.plan_requests',
        'compare.user_sessions',
        'identity.session_plan_requests',
        'identity.session_plans',

        -- 9. Workflow & HR Applications
        'workflow.automation_requests',
        'workflow.report_issues',
        'workflow.weekly_coverages',
        'workflow.requests',
        'workflow.purchase_priority_engines',
        'hr.job_applications',
        'hr.job_offers',
        'hr.job_seeker_profiles',

        -- 10. Organizations, Branches & Memberships
        'org.organization_deletion_requests',
        'org.profile_change_requests',
        'org.organization_followers',
        'org.organization_policies',
        'org.organization_reviews',
        'org.organization_social_media',
        'org.review_ratings',
        'org.user_organization_numbers',
        'org.user_organizations',
        'org.delivery_bands',
        'org.branch_institutional_works',
        'org.employee_institutional_works',
        'org.institutional_work_connections',
        'org.institutional_works',
        'org.highlight_section_items',
        'org.highlight_sections',
        'org.custom_roles',
        'org.role_permissions',
        'org.roles',
        'org.members',
        'org.branches',
        'org.organizations',

        -- 11. Background Queue Jobs
        'river_job',
        'river_leader'
    ];
BEGIN
    -- Truncate each table and restart sequences, safely skipping any table not present in the current schema
    FOREACH tbl IN ARRAY tables_to_truncate LOOP
        IF to_regclass(tbl) IS NOT NULL THEN
            EXECUTE 'TRUNCATE TABLE ' || tbl || ' RESTART IDENTITY CASCADE';
            RAISE NOTICE 'Truncated: %', tbl;
        END IF;
    END LOOP;
END $$;

-- -----------------------------------------------------------------------------
-- 12. Clean up Users & Identity:
--     Purge all test pharmacy/vendor accounts, while PRESERVING Super Admin / Staff.
-- -----------------------------------------------------------------------------
DO $$
BEGIN
    -- Delete auxiliary user activity & session history
    IF to_regclass('identity.account_deletion_requests') IS NOT NULL THEN
        DELETE FROM identity.account_deletion_requests;
    END IF;
    IF to_regclass('identity.kyc_records') IS NOT NULL THEN
        DELETE FROM identity.kyc_records;
    END IF;
    IF to_regclass('identity.user_address_histories') IS NOT NULL THEN
        DELETE FROM identity.user_address_histories;
    END IF;
    IF to_regclass('identity.user_favorites') IS NOT NULL THEN
        DELETE FROM identity.user_favorites;
    END IF;
    IF to_regclass('identity.user_session_histories') IS NOT NULL THEN
        DELETE FROM identity.user_session_histories;
    END IF;
    IF to_regclass('identity.user_sessions') IS NOT NULL THEN
        DELETE FROM identity.user_sessions;
    END IF;

    -- Delete non-staff user profiles, preferences, identities, and credentials
    IF to_regclass('profile.user_preferences') IS NOT NULL THEN
        DELETE FROM profile.user_preferences 
        WHERE user_id NOT IN (SELECT id FROM identity.users WHERE role IN ('super_admin', 'admin'));
    END IF;

    IF to_regclass('profile.user_profiles') IS NOT NULL THEN
        DELETE FROM profile.user_profiles 
        WHERE user_id NOT IN (SELECT id FROM identity.users WHERE role IN ('super_admin', 'admin'));
    END IF;

    IF to_regclass('identity.user_roles') IS NOT NULL THEN
        DELETE FROM identity.user_roles 
        WHERE user_id NOT IN (SELECT id FROM identity.users WHERE role IN ('super_admin', 'admin'));
    END IF;

    IF to_regclass('identity.user_identities') IS NOT NULL THEN
        DELETE FROM identity.user_identities 
        WHERE user_id NOT IN (SELECT id FROM identity.users WHERE role IN ('super_admin', 'admin'));
    END IF;

    IF to_regclass('identity.user_mfa') IS NOT NULL THEN
        DELETE FROM identity.user_mfa 
        WHERE user_id NOT IN (SELECT id FROM identity.users WHERE role IN ('super_admin', 'admin'));
    END IF;

    IF to_regclass('identity.user_security') IS NOT NULL THEN
        DELETE FROM identity.user_security 
        WHERE user_id NOT IN (SELECT id FROM identity.users WHERE role IN ('super_admin', 'admin'));
    END IF;

    IF to_regclass('identity.users') IS NOT NULL THEN
        DELETE FROM identity.users 
        WHERE role NOT IN ('super_admin', 'admin');
    END IF;

    RAISE NOTICE 'Users sanitized: Kept super_admin and admin accounts.';
END $$;

-- -----------------------------------------------------------------------------
-- 13. Ensure RBAC Catalogue & Permissions Version is Fresh
-- -----------------------------------------------------------------------------
DO $$
BEGIN
    IF to_regclass('identity.rbac_version') IS NOT NULL THEN
        UPDATE identity.rbac_version SET version = version + 1, updated_at = now();
    END IF;
    IF to_regclass('platform_admin.page_control_version') IS NOT NULL THEN
        UPDATE platform_admin.page_control_version SET version = version + 1, updated_at = now();
    END IF;
END $$;

COMMIT;

-- =============================================================================
-- Verification Summary Query (Run after script completion to inspect state)
-- =============================================================================
SELECT 'Catalog Products' AS entity, COUNT(*) AS count FROM catalog.products
UNION ALL
SELECT 'Catalog Brands', COUNT(*) FROM catalog.brands
UNION ALL
SELECT 'Catalog Categories', COUNT(*) FROM catalog.categories
UNION ALL
SELECT 'Billing Plans (KEPT)', COUNT(*) FROM billing.plans
UNION ALL
SELECT 'Admin Users (KEPT)', COUNT(*) FROM identity.users WHERE role IN ('super_admin', 'admin')
UNION ALL
SELECT 'Test Users (Remaining)', COUNT(*) FROM identity.users WHERE role NOT IN ('super_admin', 'admin')
UNION ALL
SELECT 'Organizations (Remaining)', COUNT(*) FROM org.organizations
UNION ALL
SELECT 'Orders (Remaining)', COUNT(*) FROM commerce.orders
UNION ALL
SELECT 'Notifications (Remaining)', COUNT(*) FROM notifications.logs;
