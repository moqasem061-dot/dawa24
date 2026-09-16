-- 110_unified_subscription_plans.up.sql
-- Unified subscription plans with organization-wide concurrent sessions, device limits, AI Gateway plan binding, and automated basic tier.

BEGIN;

-- 1. Extend billing.plans with unified limits and AI Gateway plan linkage
ALTER TABLE billing.plans
    ADD COLUMN IF NOT EXISTS max_login_sessions INT NOT NULL DEFAULT 3,
    ADD COLUMN IF NOT EXISTS max_devices INT NOT NULL DEFAULT 3,
    ADD COLUMN IF NOT EXISTS ai_plan_id TEXT NOT NULL DEFAULT 'plan-basic',
    ADD COLUMN IF NOT EXISTS is_default BOOLEAN NOT NULL DEFAULT false;

-- 2. Extend org.organizations with AI Gateway credentials
ALTER TABLE org.organizations
    ADD COLUMN IF NOT EXISTS ai_virtual_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS ai_user_id TEXT NOT NULL DEFAULT '';

-- 3. Ensure Default Basic Plan exists
INSERT INTO billing.plans (slug, name, description, price_month, price_year, duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active)
VALUES (
    'basic',
    '{"ar":"الباقة الأساسية المجانية","en":"Basic Free Plan"}'::JSONB,
    '{"ar":"الخطة الافتراضية الشاملة لكافة المنشآت والحسابات المسجلة تشمل الربط مع بوابة الذكاء الاصطناعي وجلسات متزامنة متعددة","en":"Default plan for all registered organizations with AI gateway access and concurrent sessions"}'::JSONB,
    0,
    0,
    3650,
    10,
    3,
    3,
    'plan-basic',
    true,
    true
)
ON CONFLICT (slug) DO UPDATE
SET is_default = true,
    max_login_sessions = 3,
    max_devices = 3,
    ai_plan_id = 'plan-basic',
    is_active = true;

-- 4. Seed Pro and Enterprise tiers if not present
INSERT INTO billing.plans (slug, name, description, price_month, price_year, duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active)
VALUES 
(
    'pro',
    '{"ar":"الباقة الاحترافية المتطورة","en":"Professional Plan"}'::JSONB,
    '{"ar":"خطة متقدمة للصيدليات الكبرى والموزعين مع دعم 10 جلسات متزامنة وحصة ذكاء اصطناعي مضاعفة ومقارنة خصومات","en":"Advanced plan with 10 concurrent sessions, higher AI limits and discount comparison"}'::JSONB,
    50000,
    500000,
    30,
    50,
    10,
    10,
    'plan-pro',
    false,
    true
),
(
    'enterprise',
    '{"ar":"باقة الشركات والمصانع","en":"Enterprise Suite"}'::JSONB,
    '{"ar":"خطة غير محدودة للمصانع والشركات الكبرى تشمل جلسات موسعة وتكامل كامل مع الذكاء الاصطناعي","en":"Unlimited enterprise tier for pharmaceutical manufacturers and large distribution hubs"}'::JSONB,
    150000,
    1500000,
    30,
    500,
    50,
    50,
    'plan-enterprise',
    false,
    true
)
ON CONFLICT (slug) DO UPDATE
SET max_login_sessions = EXCLUDED.max_login_sessions,
    max_devices = EXCLUDED.max_devices,
    ai_plan_id = EXCLUDED.ai_plan_id,
    is_active = true;

COMMIT;
