-- Migration 181: Grant wallet view and manage permissions to org_manager and org_pharmacist

BEGIN;

-- 1. Vendor org_manager
INSERT INTO org.role_permissions (role_id, permission_key)
SELECT r.id, p.perm
FROM org.roles r
JOIN org.organizations o ON o.id = r.organization_id
CROSS JOIN (VALUES ('vendor.wallet.view'), ('vendor.wallet.manage')) AS p(perm)
WHERE r.key = 'org_manager' AND o.type = 'vendor'
ON CONFLICT (role_id, permission_key) DO NOTHING;

-- 2. Customer org_manager
INSERT INTO org.role_permissions (role_id, permission_key)
SELECT r.id, p.perm
FROM org.roles r
JOIN org.organizations o ON o.id = r.organization_id
CROSS JOIN (VALUES ('pharmacy.wallet.view'), ('pharmacy.wallet.manage')) AS p(perm)
WHERE r.key = 'org_manager' AND o.type = 'customer'
ON CONFLICT (role_id, permission_key) DO NOTHING;

-- 3. Customer org_pharmacist
INSERT INTO org.role_permissions (role_id, permission_key)
SELECT r.id, p.perm
FROM org.roles r
JOIN org.organizations o ON o.id = r.organization_id
CROSS JOIN (VALUES ('pharmacy.wallet.view'), ('pharmacy.wallet.manage')) AS p(perm)
WHERE r.key = 'org_pharmacist' AND o.type = 'customer'
ON CONFLICT (role_id, permission_key) DO NOTHING;

COMMIT;
