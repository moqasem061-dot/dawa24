BEGIN;
DELETE FROM identity.role_permissions WHERE permission_key LIKE '%.admin';
DELETE FROM identity.permissions WHERE key LIKE '%.admin';
COMMIT;
