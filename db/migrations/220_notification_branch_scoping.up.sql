-- 220_notification_branch_scoping.up.sql
-- Add branch_id to notifications.logs to support branch-scoped notification filtering

ALTER TABLE notifications.logs
    ADD COLUMN IF NOT EXISTS branch_id BIGINT REFERENCES org.branches(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS notification_logs_branch_idx
    ON notifications.logs (branch_id)
    WHERE branch_id IS NOT NULL;
