BEGIN;

DELETE FROM identity.role_permissions
WHERE permission_key IN ('hr.admin', 'inventory.admin', 'notifications.admin', 'workflow.admin');

DELETE FROM identity.permissions
WHERE key IN ('hr.admin', 'inventory.admin', 'notifications.admin', 'workflow.admin');

COMMIT;
