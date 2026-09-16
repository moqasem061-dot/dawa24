-- 220_notification_branch_scoping.down.sql

DROP INDEX IF EXISTS notifications.notification_logs_branch_idx;
ALTER TABLE notifications.logs DROP COLUMN IF EXISTS branch_id;
