-- 110_unified_subscription_plans.down.sql
BEGIN;

ALTER TABLE billing.plans
    DROP COLUMN IF EXISTS max_login_sessions,
    DROP COLUMN IF EXISTS max_devices,
    DROP COLUMN IF EXISTS ai_plan_id,
    DROP COLUMN IF EXISTS is_default;

ALTER TABLE org.organizations
    DROP COLUMN IF EXISTS ai_virtual_key,
    DROP COLUMN IF EXISTS ai_user_id;

COMMIT;
