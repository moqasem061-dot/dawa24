BEGIN;

DELETE FROM org.role_permissions
WHERE permission_key IN ('vendor.wallet.view', 'vendor.wallet.manage')
  AND role_id IN (SELECT id FROM org.roles WHERE key = 'org_manager');

DELETE FROM org.role_permissions
WHERE permission_key IN ('pharmacy.wallet.view', 'pharmacy.wallet.manage')
  AND role_id IN (SELECT id FROM org.roles WHERE key IN ('org_manager', 'org_pharmacist'));

COMMIT;
