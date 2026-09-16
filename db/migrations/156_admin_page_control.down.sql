-- 156_admin_page_control (down)
--
-- Reversible in full: the feature is inert while every row is enabled, so
-- dropping the table simply returns the router to unconditional serving.

BEGIN;

DROP FUNCTION IF EXISTS platform_admin.bump_page_control_version();
DROP TABLE IF EXISTS platform_admin.managed_pages;
DROP TABLE IF EXISTS platform_admin.page_control_version;

COMMIT;
