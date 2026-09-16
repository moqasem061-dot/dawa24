-- Revert Migration 208
UPDATE platform_admin.system_settings
SET value = '{"value": 25, "days": 25}'::jsonb,
    description = 'Number of days a tenant on a paid subscription must wait before changing plan',
    updated_at = now()
WHERE key = 'subscription_change_cooldown_days';

UPDATE platform_admin.system_settings
SET value = '{"value": 2, "days": 2}'::jsonb,
    description = 'Minimum number of days after any subscription purchase before another change is allowed',
    updated_at = now()
WHERE key = 'subscription_change_min_days';

DELETE FROM platform_admin.system_settings WHERE key = 'subscription_change_cooldown_hours';
