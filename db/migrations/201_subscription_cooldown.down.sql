-- Migration 201: Subscription change cooldown settings (down)

DELETE FROM platform_admin.system_settings
WHERE key IN ('subscription_change_cooldown_days', 'subscription_change_min_days');

