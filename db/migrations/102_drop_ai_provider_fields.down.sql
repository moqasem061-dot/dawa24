-- Revert: Add empty provider and model keys to ai_configuration
UPDATE platform_admin.system_settings
SET value = value || '{"provider":"","model":""}'::jsonb
WHERE key = 'ai_configuration';
