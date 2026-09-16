-- Migration 208: Update subscription change cooldown to 24 hours
--
-- Cooldown setting for pharmacy and vendor plan upgrades/changes is updated from 25 days to 24 hours.

UPDATE platform_admin.system_settings
SET value = '{"value": 24, "hours": 24}'::jsonb,
    description = 'Number of hours a tenant on a paid subscription must wait before changing plan (default 24 hours)',
    updated_at = now()
WHERE key = 'subscription_change_cooldown_hours';

INSERT INTO platform_admin.system_settings (key, value, description, is_public, updated_at)
VALUES 
    ('subscription_change_cooldown_hours', '{"value": 24, "hours": 24}'::jsonb, 'Number of hours a tenant on a paid subscription must wait before changing plan', false, now())
ON CONFLICT (key) DO UPDATE
SET value = '{"value": 24, "hours": 24}'::jsonb,
    description = 'Number of hours a tenant on a paid subscription must wait before changing plan',
    updated_at = now();

UPDATE platform_admin.system_settings
SET value = '{"value": 1, "days": 1, "hours": 24}'::jsonb,
    description = 'Number of days/hours a tenant on a paid subscription must wait before changing plan (1 day / 24 hours)',
    updated_at = now()
WHERE key = 'subscription_change_cooldown_days';

UPDATE platform_admin.system_settings
SET value = '{"value": 1, "days": 1, "hours": 24}'::jsonb,
    description = 'Minimum number of days/hours after any subscription purchase before another change is allowed (1 day / 24 hours)',
    updated_at = now()
WHERE key = 'subscription_change_min_days';
