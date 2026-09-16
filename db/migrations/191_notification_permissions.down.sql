-- 191_notification_permissions.down.sql
BEGIN;

DROP INDEX IF EXISTS notifications.notification_logs_perm_idx;

ALTER TABLE notifications.logs
    DROP COLUMN IF EXISTS required_permission;

COMMIT;
