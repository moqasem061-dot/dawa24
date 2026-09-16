-- Migration 102: Strip provider and model keys from platform_admin.system_settings ai_configuration
UPDATE platform_admin.system_settings
SET value = value - 'provider' - 'model'
WHERE key = 'ai_configuration';
