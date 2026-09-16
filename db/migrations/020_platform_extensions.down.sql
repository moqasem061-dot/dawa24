BEGIN;

DROP TABLE IF EXISTS platform_admin.documents CASCADE;
DROP TABLE IF EXISTS platform_admin.contact_messages CASCADE;
DROP TABLE IF EXISTS platform_admin.languages CASCADE;
DROP TABLE IF EXISTS platform_admin.currencies CASCADE;

COMMIT;
