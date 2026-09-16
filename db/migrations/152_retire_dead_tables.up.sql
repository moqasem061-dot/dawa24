-- 152_retire_dead_tables
--
-- Twenty tables that are empty in production *and* referenced nowhere in the Go
-- or templ source. Both conditions were checked, and both had to hold: an empty
-- table that code still reads is a feature waiting for data, not dead weight,
-- and forty-seven such tables are deliberately left alone by this migration.
--
-- Three tables that met both conditions are also left alone because a live
-- table still points at them by foreign key, and removing them would mean
-- altering a table that is in use:
--
--   billing.payment_integrations         <- billing.payments
--   inventory.father_user_temparte_warehouses <- inventory.temp_warehouses
--
-- (inventory.plan_temparte_warehouses is dropped, because its only referrer,
-- inventory.user_plan_temparte_warehouses, is dropped in the same statement.)
--
-- Most of these are literal transliterations carried over from the Laravel
-- schema — "temparte" for "template", user_plan_histories alongside
-- subscription_histories — and were never wired up in the rewrite.
--
-- The down migration recreates every table with its columns, types, defaults
-- and primary key. It does not recreate their foreign keys or secondary
-- indexes: those referenced tables may themselves have changed, and an empty
-- table has nothing for them to protect. This is recorded here rather than
-- discovered later.

BEGIN;

DROP TABLE IF EXISTS
    billing.payment_histories,
    billing.plan_types,
    billing.user_plan_histories,
    identity.kyc_records,
    identity.session_plan_requests,
    identity.user_identities,
    identity.user_session_histories,
    ingest.import_batches,
    inventory.user_plan_temparte_warehouses,
    inventory.plan_temparte_warehouses,
    inventory.supplier_trackings,
    notifications.admin_notifications,
    org.user_organization_numbers,
    platform_admin.ai_providers,
    platform_admin.api_integrations,
    platform_admin.system_resources,
    profile.user_profiles,
    promo.offer_package_features,
    promo.offer_promotions,
    promo.offer_views
CASCADE;

COMMIT;
