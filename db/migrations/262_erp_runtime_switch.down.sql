-- Rollback of 262: remove the ERP runtime switch row. Code that finds no row
-- treats the integration as on.

BEGIN;

DELETE FROM platform_admin.feature_flags WHERE key = 'erp.enabled';

COMMIT;
