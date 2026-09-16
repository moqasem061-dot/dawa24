-- Migration 201: Subscription change cooldown settings
--
-- Configurable cooldown settings for subscription tier changes:
-- - subscription_change_cooldown_days: default 25 days (paid plan -> any change)
-- - subscription_change_min_days: default 2 days (minimum wait after any purchase)

INSERT INTO platform_admin.system_settings (key, value, description, is_public, updated_at)
VALUES 
    ('subscription_change_cooldown_days', '{"value": 25, "days": 25}'::jsonb, 'Number of days a tenant on a paid subscription must wait before changing plan', false, now()),
    ('subscription_change_min_days', '{"value": 2, "days": 2}'::jsonb, 'Minimum number of days after any subscription purchase before another change is allowed', false, now())
ON CONFLICT (key) DO NOTHING;
