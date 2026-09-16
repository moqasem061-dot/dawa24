-- 156_admin_page_control
--
-- Route-level enable/disable of any system page from one admin screen.
--
-- The enforcement lives in internal/platform/pagecontrol.Guard, an outermost
-- HTTP handler that wraps the whole router and answers a disabled route with the
-- same 404 an unknown route gets. This migration is structural only: it does not
-- seed the route list. internal/platform/pagecontrol.SyncDiscovered walks the
-- chi route table at boot and populates the catalogue, for the same reason
-- migration 145 does not seed identity.permissions — two sources that can drift
-- is the failure this feature is built to avoid.
--
-- What IS seeded here: the protected rows that must never be disabled, so the
-- control panel, the dashboard, the roles screen and authentication keep working
-- even if an operator disables their whole parent tree. The Guard carries a
-- second, compiled-in copy of that list, so a bad row cannot lock anyone out.

BEGIN;

-- ---------------------------------------------------------------------------
-- platform_admin.managed_pages: one row per controllable page or route root
-- ---------------------------------------------------------------------------
CREATE TABLE platform_admin.managed_pages (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    resource       TEXT NOT NULL DEFAULT 'independent',
    path           TEXT NOT NULL,
    match_mode     TEXT NOT NULL DEFAULT 'exact',
    label          JSONB NOT NULL DEFAULT '{}'::jsonb,
    description    TEXT NOT NULL DEFAULT '',
    is_enabled     BOOLEAN NOT NULL DEFAULT true,
    is_system      BOOLEAN NOT NULL DEFAULT false,
    is_lockable    BOOLEAN NOT NULL DEFAULT true,
    route_patterns TEXT[] NOT NULL DEFAULT '{}',
    source         TEXT NOT NULL DEFAULT 'manual',
    discovered_at  TIMESTAMPTZ,
    created_by     BIGINT REFERENCES identity.users (id) ON DELETE SET NULL,
    updated_by     BIGINT REFERENCES identity.users (id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ,
    CONSTRAINT managed_pages_resource_check   CHECK (resource IN ('admin', 'vendor', 'client', 'independent')),
    CONSTRAINT managed_pages_match_mode_check CHECK (match_mode IN ('exact', 'prefix')),
    CONSTRAINT managed_pages_source_check     CHECK (source IN ('discovered', 'manual')),
    CONSTRAINT managed_pages_path_shape_check CHECK (path ~ '^/[^?#[:space:]]*$' AND length(path) <= 512),
    CONSTRAINT managed_pages_no_root_prefix   CHECK (NOT (match_mode = 'prefix' AND path = '/'))
);

COMMENT ON TABLE platform_admin.managed_pages IS
    'صفحات النظام القابلة للتفعيل/التعطيل على مستوى الـ Route. الإنفاذ في internal/platform/pagecontrol.Guard.';
COMMENT ON COLUMN platform_admin.managed_pages.match_mode IS
    'exact = هذا المسار فقط؛ prefix = هذا المسار وكل ما تحته. الأطول تطابقاً يفوز.';
COMMENT ON COLUMN platform_admin.managed_pages.is_lockable IS
    'false ⇒ تُرفض محاولة التعطيل على مستوى الخدمة (مسار حيوي).';
COMMENT ON COLUMN platform_admin.managed_pages.route_patterns IS
    'أنماط chi المكتشفة تحت مسار التفعيل — للعرض والتحذير فقط، لا للإنفاذ.';

CREATE UNIQUE INDEX uq_managed_pages_path
    ON platform_admin.managed_pages (path) WHERE deleted_at IS NULL;
CREATE INDEX idx_managed_pages_enabled
    ON platform_admin.managed_pages (is_enabled) WHERE deleted_at IS NULL;
CREATE INDEX idx_managed_pages_resource
    ON platform_admin.managed_pages (resource) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Cross-process cache invalidation
-- ---------------------------------------------------------------------------
-- The in-memory engine reloads on a background timer, but a disable an operator
-- makes on one instance should reach the others in about a second, not twenty.
-- Same shape as identity.rbac_version: a counter bumped inside the same
-- transaction as the write, plus a NOTIFY so a listening process re-reads at
-- once. The timer stays as the safety net.
CREATE TABLE platform_admin.page_control_version (
    scope_key  TEXT PRIMARY KEY DEFAULT 'global',
    version    BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO platform_admin.page_control_version (scope_key, version) VALUES ('global', 1);

CREATE OR REPLACE FUNCTION platform_admin.bump_page_control_version()
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE
    v BIGINT;
BEGIN
    UPDATE platform_admin.page_control_version
       SET version = version + 1, updated_at = now()
     WHERE scope_key = 'global'
    RETURNING version INTO v;
    PERFORM pg_notify('pagecontrol_changed', v::text);
    RETURN v;
END;
$$;

COMMENT ON FUNCTION platform_admin.bump_page_control_version() IS
    'يزيد عدّاد إبطال كاش صفحات النظام ويرسل NOTIFY؛ يُستدعى داخل نفس معاملة أي تعديل على managed_pages.';

-- ---------------------------------------------------------------------------
-- Protected rows: never lockable, never deleted
-- ---------------------------------------------------------------------------
INSERT INTO platform_admin.managed_pages
    (resource, path, match_mode, label, description, is_enabled, is_system, is_lockable, source)
VALUES
    ('admin', '/admin/system-pages', 'prefix',
        '{"ar":"التحكم في صفحات النظام","en":"System pages"}'::jsonb,
        'لوحة التحكم في الصفحات نفسها — لا تُعطَّل.', true, true, false, 'manual'),
    ('admin', '/admin/dashboard', 'prefix',
        '{"ar":"لوحة الإدارة الرئيسية","en":"Admin dashboard"}'::jsonb,
        'مدخل لوحة الإدارة — مخرج الطوارئ.', true, true, false, 'manual'),
    ('admin', '/admin/roles', 'prefix',
        '{"ar":"الأدوار والصلاحيات","en":"Roles & permissions"}'::jsonb,
        'إدارة الأدوار — مخرج الطوارئ.', true, true, false, 'manual'),
    ('independent', '/auth', 'prefix',
        '{"ar":"تسجيل الدخول والخروج","en":"Authentication"}'::jsonb,
        'الدخول والخروج واستعادة كلمة المرور.', true, true, false, 'manual'),
    ('independent', '/health', 'prefix',
        '{"ar":"فحوص الصحة","en":"Health checks"}'::jsonb,
        'فحوص الأوركستريتور.', true, true, false, 'manual');

COMMIT;
