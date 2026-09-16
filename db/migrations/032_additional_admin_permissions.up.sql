BEGIN;

-- Admin permissions for remaining modules: hr, inventory, notifications, workflow
INSERT INTO identity.permissions (key, name, module, description) VALUES
  ('hr.admin', '{"ar":"إدارة الموارد البشرية","en":"HR Administration"}', 'hr', 'Cross-tenant employee and compensation management'),
  ('inventory.admin', '{"ar":"إدارة المخزون","en":"Inventory Administration"}', 'inventory', 'Cross-tenant warehouse and transfer management'),
  ('notifications.admin', '{"ar":"إدارة الإشعارات","en":"Notifications Administration"}', 'notifications', 'Broadcasts and template management'),
  ('workflow.admin', '{"ar":"إدارة سير العمل","en":"Workflow Administration"}', 'workflow', 'Cross-tenant issues and purchasing workflow management')
ON CONFLICT (key) DO NOTHING;

-- Grant to admin and super_admin roles
INSERT INTO identity.role_permissions (role_key, permission_key)
SELECT r.key, p.key
FROM (SELECT unnest(ARRAY['admin', 'super_admin']) AS key) r
CROSS JOIN identity.permissions p
WHERE p.key IN ('hr.admin', 'inventory.admin', 'notifications.admin', 'workflow.admin')
ON CONFLICT (role_key, permission_key) DO NOTHING;

COMMIT;
