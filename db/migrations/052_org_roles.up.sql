-- Migration 052: Per-Organization Custom Roles & Employee HR Parity
-- Establishes org.roles, org.role_permissions, and expands org.members with employee attributes.

BEGIN;

-- 1. Organization Roles
CREATE TABLE IF NOT EXISTS org.roles (
    id              BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    key             TEXT NOT NULL,
    name            JSONB NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    is_system       BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_org_roles_key UNIQUE (organization_id, key)
);

CREATE INDEX IF NOT EXISTS idx_org_roles_org ON org.roles (organization_id);

-- 2. Organization Role Permissions
CREATE TABLE IF NOT EXISTS org.role_permissions (
    role_id         BIGINT NOT NULL REFERENCES org.roles(id) ON DELETE CASCADE,
    permission_key  TEXT NOT NULL,
    PRIMARY KEY (role_id, permission_key)
);

-- 3. Expand org.members with Employee Fields
ALTER TABLE org.members
    ADD COLUMN IF NOT EXISTS employee_code    TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS job_title        TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS base_salary      NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    ADD COLUMN IF NOT EXISTS variable_salary  NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    ADD COLUMN IF NOT EXISTS org_role_id      BIGINT REFERENCES org.roles(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_org_members_role ON org.members (org_role_id);

ALTER TABLE org.roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.role_permissions ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'roles_tenant_isolation') THEN
        CREATE POLICY roles_tenant_isolation ON org.roles
            USING (true)
            WITH CHECK (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'role_permissions_tenant_isolation') THEN
        CREATE POLICY role_permissions_tenant_isolation ON org.role_permissions
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- 4. Seed Standard Template Roles for Existing Organizations
DO $$
DECLARE
    o RECORD;
    v_role_id BIGINT;
BEGIN
    FOR o IN SELECT id FROM org.organizations LOOP
        -- Owner
        INSERT INTO org.roles (organization_id, key, name, description, is_system)
        VALUES (o.id, 'org_owner', '{"ar":"مالك المنشأة","en":"Organization Owner"}'::jsonb, 'صلاحيات كاملة على المنشأة والفروع والموظفين', true)
        ON CONFLICT DO NOTHING RETURNING id INTO v_role_id;

        -- Manager
        INSERT INTO org.roles (organization_id, key, name, description, is_system)
        VALUES (o.id, 'org_manager', '{"ar":"مدير عام / مدير فرع","en":"Branch / Operations Manager"}'::jsonb, 'إدارة المخزون والمبيعات والطلبيات اليومية', true)
        ON CONFLICT DO NOTHING;

        -- Pharmacist
        INSERT INTO org.roles (organization_id, key, name, description, is_system)
        VALUES (o.id, 'org_pharmacist', '{"ar":"صيدلي مسؤول","en":"Responsible Pharmacist"}'::jsonb, 'طلب الأدوية ومتابعة التوريدات والصرف', true)
        ON CONFLICT DO NOTHING;

        -- Warehouse Manager
        INSERT INTO org.roles (organization_id, key, name, description, is_system)
        VALUES (o.id, 'org_warehouse', '{"ar":"أمين مخزن وتوزيع","en":"Warehouse Keeper"}'::jsonb, 'استلام وتخزين الشحنات وضبط الأرصدة', true)
        ON CONFLICT DO NOTHING;

        -- Accountant
        INSERT INTO org.roles (organization_id, key, name, description, is_system)
        VALUES (o.id, 'org_accountant', '{"ar":"محاسب مالي","en":"Financial Accountant"}'::jsonb, 'إدارة المحفظة والفواتير والتسويات المالية', true)
        ON CONFLICT DO NOTHING;
    END LOOP;
END $$;

COMMIT;
