-- 145_rbac_foundation
--
-- Turns the three half-built role systems into one.
--
-- What was here before: identity.permissions held 112 rows seeded by three
-- earlier migrations; the admin sidebar gated links on six keys that were
-- never among them, so those links were invisible to every account except
-- super_admin and no role could be granted them. org.roles was created by
-- migration 052 and had zero rows in production — the vendor and pharmacy
-- "roles" screens were static markup. org.members carried both a system
-- role_key and an unused org_role_id. Effective permissions were resolved from
-- role_key alone, so a custom organization role, had anyone created one,
-- would have granted nothing.
--
-- What this migration does is structural only. It does not seed the
-- catalogue: internal/platform/rbac is the source of truth for which
-- permissions exist, and the application syncs this table from it at boot
-- (rbac.Sync). Seeding here as well would give two sources that drift, which
-- is the failure this whole change exists to end.

BEGIN;

-- ---------------------------------------------------------------------------
-- identity.permissions becomes a mirror of the Go catalogue
-- ---------------------------------------------------------------------------
-- The extra columns are what the role editor needs to render a matrix: which
-- dashboards may grant a permission, which section it belongs to, whether it
-- is a page or an action within one, and which sidebar item it reveals.
ALTER TABLE identity.permissions
    ADD COLUMN IF NOT EXISTS group_key  TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS kind       TEXT NOT NULL DEFAULT 'action',
    ADD COLUMN IF NOT EXISTS scopes     TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS nav_key    TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS synced_at  TIMESTAMPTZ;

COMMENT ON TABLE identity.permissions IS
    'مرآة لكتالوج الصلاحيات المعرّف في الكود (internal/platform/rbac). لا تُعدّل يدوياً.';
COMMENT ON COLUMN identity.permissions.scopes IS
    'لوحات التحكم التي يمكن منح هذه الصلاحية داخلها: admin / vendor / pharmacy';
COMMENT ON COLUMN identity.permissions.nav_key IS
    'مفتاح عنصر القائمة الجانبية الذي تكشفه هذه الصلاحية';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'permissions_kind_check'
    ) THEN
        ALTER TABLE identity.permissions
            ADD CONSTRAINT permissions_kind_check CHECK (kind IN ('page', 'action'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_permissions_scopes ON identity.permissions USING GIN (scopes);
CREATE INDEX IF NOT EXISTS idx_permissions_group ON identity.permissions (group_key, sort_order);

-- ---------------------------------------------------------------------------
-- identity.roles gains the staff flag and an edit trail
-- ---------------------------------------------------------------------------
-- is_staff replaces the hardcoded list Session.IsStaff carried
-- ('super_admin','admin','support','developer'). With the list in code, a
-- super admin could create a moderator role but its holders could not reach
-- /admin/* — the audience gate did not know the role existed. The flag is the
-- database answering the question the code used to answer from memory.
ALTER TABLE identity.roles
    ADD COLUMN IF NOT EXISTS is_staff   BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS created_by BIGINT REFERENCES identity.users (id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

COMMENT ON COLUMN identity.roles.is_staff IS
    'هل يصل حاملو هذا الدور إلى لوحة الإدارة /admin/*';
COMMENT ON COLUMN identity.roles.deleted_at IS
    'الحذف الناعم؛ الدور المحذوف لا يمنح أي صلاحية';

-- The four roles the platform shipped with are staff roles. Custom roles a
-- super admin creates later declare their own flag.
UPDATE identity.roles
   SET is_staff = true
 WHERE key IN ('super_admin', 'admin', 'support', 'developer');

CREATE INDEX IF NOT EXISTS idx_identity_roles_staff
    ON identity.roles (is_staff) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- org.roles becomes a real, per-company, editable role
-- ---------------------------------------------------------------------------
ALTER TABLE org.roles
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS created_by BIGINT REFERENCES identity.users (id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS is_owner   BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN org.roles.is_owner IS
    'دور المالك: يملك كل صلاحيات لوحة المنشأة تلقائياً ولا يُحذف';

CREATE INDEX IF NOT EXISTS idx_org_roles_org
    ON org.roles (organization_id) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- org.role_permissions: real tenant isolation, and a foreign key to the
-- catalogue so a permission that stops existing stops granting
-- ---------------------------------------------------------------------------
-- The policy shipped in migration 052 had USING (true) — it read as isolation
-- and enforced nothing. The rows are reachable only through a role, so the
-- role's organization is the tenant.
DROP POLICY IF EXISTS role_permissions_tenant_isolation ON org.role_permissions;
CREATE POLICY role_permissions_tenant_isolation ON org.role_permissions
    USING (
        EXISTS (
            SELECT 1 FROM org.roles r
             WHERE r.id = org.role_permissions.role_id
               AND platform.tenant_visible(r.organization_id)
        )
    );

-- Grants naming a permission that the catalogue no longer declares are dead
-- weight that reads as access. Cascading from the mirror removes them at the
-- moment the declaration goes.
DELETE FROM org.role_permissions rp
 WHERE NOT EXISTS (
    SELECT 1 FROM identity.permissions p WHERE p.key = rp.permission_key
 );

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'org_role_permissions_permission_fkey'
    ) THEN
        ALTER TABLE org.role_permissions
            ADD CONSTRAINT org_role_permissions_permission_fkey
            FOREIGN KEY (permission_key) REFERENCES identity.permissions (key) ON DELETE CASCADE;
    END IF;
END $$;

-- ---------------------------------------------------------------------------
-- org.members: the custom role link becomes usable
-- ---------------------------------------------------------------------------
-- org_role_id has existed since migration 061 and nothing read it. Effective
-- permissions now come from the union of the system role_key and this custom
-- role, so a member can hold a starter role plus a company-specific one.
CREATE INDEX IF NOT EXISTS idx_org_members_org_role
    ON org.members (org_role_id) WHERE org_role_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_org_members_user_org
    ON org.members (user_id, organization_id) WHERE status = 'active';

COMMENT ON COLUMN org.members.org_role_id IS
    'الدور المخصص داخل المنشأة (org.roles) — صلاحياته تُضاف إلى صلاحيات role_key';

-- ---------------------------------------------------------------------------
-- Cache invalidation across processes
-- ---------------------------------------------------------------------------
-- Effective permissions are cached, because resolving them joins four tables
-- on every request. A cached grant that outlives the role change that revoked
-- it is a security hole, and Redis invalidation alone cannot close it: a
-- second application instance holds its own cache and never saw the write.
--
-- This counter is bumped in the same transaction as any role, membership or
-- grant change. A resolver compares the version it cached against the current
-- one and re-reads when they differ, so revocation is visible to every process
-- on the next request regardless of which one performed the write.
CREATE TABLE IF NOT EXISTS identity.rbac_version (
    scope_key  TEXT PRIMARY KEY,
    version    BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE identity.rbac_version IS
    'عدّاد إبطال ذاكرة الصلاحيات: "platform" للأدوار العامة و"org:<id>" لكل منشأة';

INSERT INTO identity.rbac_version (scope_key, version)
VALUES ('platform', 1)
ON CONFLICT (scope_key) DO NOTHING;

CREATE OR REPLACE FUNCTION identity.bump_rbac_version(p_scope TEXT)
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE
    v BIGINT;
BEGIN
    INSERT INTO identity.rbac_version (scope_key, version, updated_at)
    VALUES (p_scope, 1, now())
    ON CONFLICT (scope_key)
    DO UPDATE SET version = identity.rbac_version.version + 1, updated_at = now()
    RETURNING version INTO v;
    RETURN v;
END;
$$;

COMMENT ON FUNCTION identity.bump_rbac_version(TEXT) IS
    'يزيد عدّاد الإبطال؛ يُستدعى داخل نفس معاملة أي تعديل على الأدوار أو العضويات';

COMMIT;
