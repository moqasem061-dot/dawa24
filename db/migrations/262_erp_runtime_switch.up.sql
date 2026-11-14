-- Migration 262: ERP runtime switch.
-- The deployment switch (ERP_API_ENABLED) decides whether the integration
-- exists on a deployment. This row lets an admin turn an existing integration
-- off and on from the settings page without a restart. It defaults to on, so
-- applying the migration changes nothing.

BEGIN;

INSERT INTO platform_admin.feature_flags (key, name, description, is_enabled) VALUES
  ('erp.enabled',
   '{"ar":"تكامل الأنظمة (ERP)","en":"Systems integration (ERP)"}'::jsonb,
   '{"ar":"تشغيل أو إيقاف تكامل الأنظمة لكل الصيدليات والموردين","en":"Turns the ERP integration on or off for every pharmacy and supplier"}'::jsonb,
   true)
ON CONFLICT (key) DO NOTHING;

COMMIT;
