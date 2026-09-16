-- Reverse 145_rbac_foundation.
--
-- The catalogue columns are dropped, which discards the scope and grouping of
-- every permission. The rows themselves stay: identity.role_permissions
-- references them, and dropping the grants a role holds is not something a
-- schema rollback should do silently.

BEGIN;

DROP FUNCTION IF EXISTS identity.bump_rbac_version(TEXT);
DROP TABLE IF EXISTS identity.rbac_version;

DROP INDEX IF EXISTS org.idx_org_members_user_org;
DROP INDEX IF EXISTS org.idx_org_members_org_role;

ALTER TABLE org.role_permissions
    DROP CONSTRAINT IF EXISTS org_role_permissions_permission_fkey;

DROP POLICY IF EXISTS role_permissions_tenant_isolation ON org.role_permissions;
CREATE POLICY role_permissions_tenant_isolation ON org.role_permissions USING (true);

DROP INDEX IF EXISTS org.idx_org_roles_org;
ALTER TABLE org.roles
    DROP COLUMN IF EXISTS is_owner,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS updated_at;

DROP INDEX IF EXISTS identity.idx_identity_roles_staff;
ALTER TABLE identity.roles
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS is_staff;

DROP INDEX IF EXISTS identity.idx_permissions_group;
DROP INDEX IF EXISTS identity.idx_permissions_scopes;
ALTER TABLE identity.permissions
    DROP CONSTRAINT IF EXISTS permissions_kind_check;
ALTER TABLE identity.permissions
    DROP COLUMN IF EXISTS synced_at,
    DROP COLUMN IF EXISTS sort_order,
    DROP COLUMN IF EXISTS nav_key,
    DROP COLUMN IF EXISTS scopes,
    DROP COLUMN IF EXISTS kind,
    DROP COLUMN IF EXISTS group_key;

COMMIT;
