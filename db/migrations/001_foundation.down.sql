-- Deliberately does not drop extensions: they may be shared with other
-- databases on the same instance and dropping them is not this migration's
-- business.
BEGIN;
DROP TABLE IF EXISTS platform.settings CASCADE;
DROP TABLE IF EXISTS platform.audit_log CASCADE;
DROP FUNCTION IF EXISTS platform.normalize_arabic(TEXT);
DROP FUNCTION IF EXISTS platform.touch_updated_at();
DROP FUNCTION IF EXISTS platform.tenant_visible(BIGINT);
DROP FUNCTION IF EXISTS platform.is_system();
DROP FUNCTION IF EXISTS platform.current_org_id();
DROP SCHEMA IF EXISTS ai CASCADE;
DROP SCHEMA IF EXISTS hr CASCADE;
DROP SCHEMA IF EXISTS workflow CASCADE;
DROP SCHEMA IF EXISTS ingest CASCADE;
DROP SCHEMA IF EXISTS billing CASCADE;
DROP SCHEMA IF EXISTS promo CASCADE;
DROP SCHEMA IF EXISTS commerce CASCADE;
DROP SCHEMA IF EXISTS inventory CASCADE;
DROP SCHEMA IF EXISTS catalog CASCADE;
DROP SCHEMA IF EXISTS org CASCADE;
DROP SCHEMA IF EXISTS profile CASCADE;
DROP SCHEMA IF EXISTS identity CASCADE;
DROP SCHEMA IF EXISTS platform CASCADE;
COMMIT;
