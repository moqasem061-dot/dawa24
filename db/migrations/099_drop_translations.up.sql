-- 099_drop_translations (up)
--
-- platform_admin.translations was written by exactly one screen (the admin
-- translations manager) and read by nothing. Every user-facing string in the
-- platform is an inline bilingual branch in the templates, so an administrator
-- could spend an hour editing overrides that never appeared anywhere.
--
-- The platform stays bilingual: internal/shared/i18n and the lang/dir plumbing
-- are untouched. Only the editable-override store goes.
--
-- The screen was gated by platform.settings.manage, which still gates the rest
-- of the settings surface, so no permission is dropped here.
--
-- PLAN_V7 Phase 3 Task 3.1.

BEGIN;

DROP TABLE IF EXISTS platform_admin.translations;

COMMIT;
