# خطة تنفيذ: التحكم في صفحات النظام — Admin Page Control

الحالة: مسودة تنفيذ · المالك: فريق المنصة · التاريخ: 2026‑08‑31 · الترقيم المقترح للهجرة: `156_admin_page_control`

---

## 0. ملخص تنفيذي

ميزة تتيح لموظفي المنصة تفعيل/تعطيل أي صفحة على مستوى الـ Route من مكان مركزي واحد في لوحة الإدارة.
التعطيل يُطبَّق **Server‑side** كطبقة Middleware خارجية تسبق المصادقة، فترجع `404 Not Found`
لأي مستخدم (زائر، عميل، مورّد، أدمن) دون تسريب أن الصفحة "معطّلة". النظام **Dynamic**:
الـ Routes تُكتشف تلقائيًا من جدول توجيه chi عند الإقلاع، والأدمن يضيف صفحات مخصّصة (Independent
مثل `/about`) بتحديد الـ Path والـ Resource فقط — دون إدخال يدوي مسبق لكل صفحات النظام في قاعدة البيانات.

الميزة مدمجة في نظام **Roles & Permissions** الحالي عبر مفاتيح صلاحية جديدة
(`platform.page_control.view/create/update/delete`) تُزامَن تلقائيًا إلى `identity.permissions`
ويلتقطها محرّر الأدوار الحالي دون تعديل.

المبدأ الحاكم: الميزة **إضافية بالكامل وخاملة افتراضيًا** — لا شيء يُحجب حتى يقلب أدمن مفتاحًا،
وهناك مسارات محميّة لا يمكن تعطيلها إطلاقًا (منع قفل الأدمن خارج النظام).

---

## 1. فهم المنصة الحالية (ما تم تحليله)

### 1.1 البنية العامة

| المحور | الواقع الحالي |
|---|---|
| اللغة/الإطار | Go 1.26، `go-chi/chi/v5`، قوالب `a-h/templ` مُولّدة إلى `_templ.go`، `pgx/v5` على PostgreSQL |
| الشكل المعماري | Modular monolith: `internal/modules/*` (لكل موديول `domain/service/repository/http/postgres`)، مع `internal/platform/*` كأرضية مشتركة و`internal/ui/*` لطبقة العرض SSR |
| نقطة تركيب الراوتر | `cmd/server/main.go → newRouter()` ثم `mountModuleRoutes()` في `cmd/server/routes.go` |
| Middleware عامة (الترتيب) | `RequestID → Recover → Logger → SecurityHeaders → Locale → Compress(5)` ثم مسارات `/health` `/ready` `/api/v1/status` ثم `NotFound/MethodNotAllowed` ثم `mountModuleRoutes` |
| 404 الحالي | `r.NotFound` يرجع مغلّف JSON موحّد؛ و`authctx.notFound` يرجع صفحة HTML صغيرة (RTL) |
| القيود الهندسية | كل ملف Go ≤ 400 سطر (فحص `check-file-size` سقفه 0)؛ ممنوع نصوص عربية Hard‑coded في Go (تُضاف مفاتيح إلى `internal/shared/i18n`)؛ ممنوع CDN في القوالب |
| الهجرات | `db/migrations/NNN_name.up.sql` + `.down.sql`، تُشغَّل بـ `go run ./cmd/cli migrate`، آخر نسخة مطبّقة = **155** |
| تعدّد المستأجرين | التطبيق يتصل بـ PostgreSQL كـ superuser، فـ RLS شبه معطّل؛ التفويض يقع بالكامل في Middleware داخل Go |

### 1.2 نظام الصلاحيات (RBAC) — `internal/platform/rbac`

- **مصدر الحقيقة في الكود**: كتالوج الصلاحيات مُعلَن في Go (`catalog.go` + `catalog_admin.go` + `nav_admin.go` …).
  جدول `identity.permissions` **مرآة** فقط، تُزامَن عند الإقلاع بـ `rbac.Sync` (حذف أي مفتاح لم يعد معلَنًا،
  مع Cascade على `identity.role_permissions` و`org.role_permissions`).
- **الصلاحية** (`rbac.Permission`): `Key` بصيغة `module.resource.action`، `Kind` = `page` أو `action`،
  `Scopes` (admin/vendor/pharmacy)، `Implies` (فعل يتضمّن صفحته)، `Nav` (مفتاح عنصر القائمة الذي يكشفه).
- **بوابات المسارات**:
  - `authctx.RequireStaff` — بوابة جمهور `/admin/*` (غير الموظّف → 404).
  - `authctx.RequirePagePermission("key"...)` — بوابة صفحة HTML إدارية، ترجع **404** لا 403 (عدم تسريب وجود المسار).
  - `authctx.RequireAPIPermission` / `RequireTenantPagePermission` / `RequireAPITenantPermission` — نظائرها للـ API والمستأجر.
  - لا يوجد أي Bypass بالاسم؛ `super_admin` يملك الكتالوج كله عبر `Owner`.
- **إبطال الكاش عبر العمليات**: جدول عدّاد `identity.rbac_version` + دالة `identity.bump_rbac_version(scope)`
  تُستدعى داخل نفس معاملة أي تعديل؛ `rbac.Resolver` يقارن النسخة كل 5 ثوانٍ (TTL) ويعيد القراءة.
- **اختبارات حارسة قائمة** يجب ألّا تُكسر:
  - `TestEveryRouteGateNamesADeclaredPermission` — كل بوابة تسمّي صلاحية معلَنة.
  - `TestNoDashboardRouteIsRegisteredOutsideAGate` — كل مسار `/admin` داخل بوابة.
  - `TestEverySidebarPermissionGatesARoute` + `TestEverySidebarItemHasAGrantablePermission`.
  - `TestUIRoutesAreAudienceGated` / `TestAdminUIRoutesRequirePagePermission`.
  - `TestRepositorySQLMatchesMigrations` / `schema_consistency_test.go` — SQL في مستودعات Go يطابق الهجرات.

### 1.3 سابقة مباشرة: `internal/platform/features`

محرّك Feature Flags في الذاكرة، يعيد التحميل كل 60 ثانية في الخلفية، جدول `platform_admin.feature_flags`،
و`features.Require(key)` وهو **Middleware chi يرجع 404 عند تعطيل الميزة**. هذا بالضبط نموذج التصميم لمفتاح
إيقاف الصفحات، لكن مفتاحه ميزة لا Route. سنبني نظيرًا موازيًا `internal/platform/pagecontrol` بمفاتيح Path.

### 1.4 مخطط قاعدة البيانات ذو الصلة

- Schema `platform_admin` هو موطن الإعداد العام: `feature_flags`, `system_settings`, `content_blocks`,
  `policies`, `error_logs`, `sql_logs` …
- Schema `identity`: `permissions`, `roles`, `role_permissions`, `rbac_version`.
- Schema `platform`: `audit_log`, `translations`.

### 1.5 تصنيف الـ Resources (لوحات التحكم)

| Resource | البادئة | البوابة |
|---|---|---|
| Admin Dashboard | `/admin/*` | `RequireStaff` + `RequirePagePermission` |
| Vendor Dashboard | `/vendor/*` | `RequireVendor` + `RequireTenantPagePermission` |
| Client Dashboard | `/customer/*` | `RequireCustomer` + `RequireTenantPagePermission` |
| Independent / Public | كل ما عدا ذلك (`/`, `/about`, `/offers`, `/jobs`, `/compare`, `/auth/*` …) | `RegisterPublicRoutes` (OptionalAuth فقط) |

---

## 2. تصميم قاعدة البيانات

### 2.1 الجدول الرئيسي `platform_admin.managed_pages`

```sql
CREATE TABLE platform_admin.managed_pages (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    resource       TEXT NOT NULL DEFAULT 'independent'
                     CHECK (resource IN ('admin','vendor','client','independent')),
    path           TEXT NOT NULL,                 -- مُطبَّع: يبدأ بـ '/'، بلا '/' في النهاية، بلا Query
    match_mode     TEXT NOT NULL DEFAULT 'exact'
                     CHECK (match_mode IN ('exact','prefix')),
    label          JSONB NOT NULL DEFAULT '{}'::jsonb,   -- {"ar":"...","en":"..."}
    description    TEXT NOT NULL DEFAULT '',
    is_enabled     BOOLEAN NOT NULL DEFAULT true,
    is_system      BOOLEAN NOT NULL DEFAULT false,   -- صف مُكتشَف/مصنّف كحيوي؛ لا يُحذف
    is_lockable    BOOLEAN NOT NULL DEFAULT true,    -- false ⇒ يُرفض التعطيل على مستوى الخدمة
    route_patterns TEXT[] NOT NULL DEFAULT '{}',     -- أنماط chi المكتشفة تحت هذا المسار (للعرض فقط)
    source         TEXT NOT NULL DEFAULT 'manual'
                     CHECK (source IN ('discovered','manual')),
    discovered_at  TIMESTAMPTZ,
    created_by     BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    updated_by     BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_managed_pages_path
    ON platform_admin.managed_pages (path) WHERE deleted_at IS NULL;
CREATE INDEX idx_managed_pages_enabled
    ON platform_admin.managed_pages (is_enabled) WHERE deleted_at IS NULL;
CREATE INDEX idx_managed_pages_resource
    ON platform_admin.managed_pages (resource) WHERE deleted_at IS NULL;

COMMENT ON TABLE platform_admin.managed_pages IS
  'صفحات النظام القابلة للتفعيل/التعطيل على مستوى الـ Route. الإنفاذ في pagecontrol.Guard.';
```

قواعد إضافية على مستوى `CHECK` أو الخدمة:
- يُمنع `match_mode='prefix' AND path='/'` (تعطيل الجذر كبادئة يقتل الموقع كله).
- `path` يطابق `^/[^?#\s]*$` وطوله ≤ 512.

### 2.2 عدّاد الإبطال عبر العمليات

```sql
CREATE TABLE platform_admin.page_control_version (
    scope_key  TEXT PRIMARY KEY DEFAULT 'global',
    version    BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO platform_admin.page_control_version (scope_key, version) VALUES ('global', 1);

CREATE OR REPLACE FUNCTION platform_admin.bump_page_control_version()
RETURNS BIGINT LANGUAGE plpgsql AS $$
DECLARE v BIGINT;
BEGIN
    UPDATE platform_admin.page_control_version
       SET version = version + 1, updated_at = now()
     WHERE scope_key = 'global'
    RETURNING version INTO v;
    PERFORM pg_notify('pagecontrol_changed', v::text);
    RETURN v;
END $$;
```

نفس فلسفة `identity.bump_rbac_version`: يُستدعى **داخل نفس معاملة** أي كتابة على `managed_pages`.
`pg_notify` يقصّر زمن الانتشار بين نسخ التطبيق إلى ~ثانية؛ والـ polling يبقى شبكة أمان.

### 2.3 التدقيق (Audit)

كل كتابة (إنشاء/تعديل/تفعيل/تعطيل/حذف) تُسجَّل في `platform.audit_log` الحالي عبر
`database/audit.go`: الفاعل، المسار، الحالة قبل/بعد، السبب. لا جدول تدقيق جديد.

### 2.4 ما لا نفعله

- **لا Seeding لكامل قائمة المسارات في الهجرة.** الاكتشاف يتكفّل بذلك عند الإقلاع (نفس فلسفة تعليق الهجرة 145:
  مصدرَان يتباعدان = العطل الذي نتفاداه). الهجرة تبذر فقط الصفوف المحميّة.

---

## 3. العلاقات والـ Resources

- `managed_pages.created_by / updated_by → identity.users(id)` (ON DELETE SET NULL).
- لا مفتاح أجنبي إلى `identity.permissions`: التحكم بالصفحة أفقي عالمي، ليس صلاحية بحد ذاته؛
  الصلاحيات تحكم **من يدير** الجدول لا محتواه.
- `resource` تصنيف مُستخدَم فعليًا: تبويب الواجهة، الاكتشاف يستنتجه من البادئة، عمليات التعطيل الجماعي لكل Resource،
  وبذور المسارات المحميّة تُعرَّف لكل Resource.
- الاكتشاف يربط كل صف بـ `route_patterns[]` (مثلاً `/admin/users` ⇒ `['/admin/users','/admin/users/{id}','/admin/users/{id}/edit']`) — للعرض والتحذيرات فقط، لا للإنفاذ.

---

## 4. نظام Roles & Permissions المطلوب

### 4.1 مفاتيح جديدة في `internal/platform/rbac/catalog_admin.go`

ضمن مجموعة الإعدادات `gAdminSettings`:

```go
adminPage("platform.page_control.view",   gAdminSettings, "system_pages",
    "التحكم في صفحات النظام", "System pages"),
adminAct("platform.page_control.create",  gAdminSettings,
    "إضافة صفحة نظام", "Add system page", "platform.page_control.view"),
adminAct("platform.page_control.update",  gAdminSettings,
    "تفعيل/تعطيل صفحات النظام", "Toggle system pages", "platform.page_control.view"),
adminAct("platform.page_control.delete",  gAdminSettings,
    "حذف صفحة نظام مخصّصة", "Delete custom system page", "platform.page_control.view"),
```

- `Scopes: [ScopeAdmin]` فقط.
- تُزامَن تلقائيًا إلى `identity.permissions` عبر `rbac.Sync` عند الإقلاع؛ محرّر الأدوار (`rbac.Matrix(ScopeAdmin)`)
  يعرضها دون تعديل؛ `super_admin` (Owner) يحصل عليها فورًا.
- **حجب من دور `admin` الأساسي**: تُضاف `platform.page_control.create/update/delete` إلى خريطة `withheld`
  في `adminRoleGrants()` (مثل `platform.developer.sql`)، فتبقى حصرًا لـ `super_admin` أو دور مخصّص يمنحه super admin.
  تبقى `.view` متاحة لـ `admin`.

### 4.2 عنصر القائمة الجانبية — `internal/platform/rbac/nav_admin.go`

ضمن قسم `settings`:

```go
{Key: "system_pages", Href: "/admin/system-pages", Icon: "layers",
    NameAr: "التحكم في صفحات النظام", NameEn: "System pages",
    Perm: "platform.page_control.view"},
```

### 4.3 لماذا لا نجعلها "صلاحية لكل صفحة"

المطلوب "صلاحية خاصة بالميزة" — وهي المفاتيح الأربعة أعلاه. التحكم في *أي صفحة بعينها* آلية بيانات
(`is_enabled`) لا مفتاح صلاحية، وإلا انفجر الكتالوج بمئات المفاتيح وتعارض مع اختبار
`TestEverySidebarItemHasAGrantablePermission`. الفصل: **RBAC يقرّر من يدير المفتاح؛ `managed_pages` تقرّر حالة الصفحة.**

---

## 5. اكتشاف وإدارة الـ Routes (Dynamic Routing)

### 5.1 الاكتشاف عند الإقلاع

بعد `mountModuleRoutes(r, …)` في `newRouter`، وقبل إرجاع الـ handler:

```go
pagecontrol.SyncDiscovered(ctx, store, r) // r يحقّق chi.Routes
```

- `pagecontrol.DiscoverRoutes(routes chi.Routes) []Candidate` تمشي الشجرة عبر `chi.Walk`،
  تجمع أنماط المسارات + توابعها.
- لكل نمط: يُشتق **مسار تفعيل مقترح** = الجزء الساكن من النمط حتى أول `{` (مثال `/admin/users/{id}/edit` ⇒ `/admin/users`).
- تُجمَّع الأنماط تحت مسار التفعيل، ويُستنتج `resource` من البادئة.
- `SyncDiscovered` تُدرِج الصفوف الناقصة بـ `source='discovered', is_enabled=true, discovered_at=now()`،
  وتُحدِّث `route_patterns[]` للموجود، و**لا تلمس أبدًا** `is_enabled` أو `label` أو `is_lockable` لصف موجود
  (قرار الأدمن مقدّس). صف مُكتشف اختفى نمطه → يبقى (لا حذف تلقائي) ويُعلَّم `discovered_at` قديمًا للعرض.

### 5.2 الإضافة اليدوية

الأدمن يضيف صفًا بـ `source='manual'`: `path` (مُطبَّع)، `match_mode`، `resource` (مقترَح تلقائيًا وقابل للتعديل)،
`label` ar/en، `description`. لا حاجة لوجود Route فعلي (يدعم صفحات مستقبلية).

### 5.3 التطبيع `pagecontrol.NormalizePath`

1. قصّ الفراغات، إسقاط أي `?...` أو `#...`.
2. ضمان بادئة `/`؛ طيّ `//` المتكرّر.
3. إزالة `/` النهائية عدا الجذر `/`.
4. لا تحويل حالة الأحرف (مسارات chi هنا صغيرة الحروف أصلًا)؛ تُوفَّر مقارنة حسّاسة للحالة.
5. رفض إن خالف `^/[^?#\s]*$`.

نفس الدالة تُطبَّق على مسار الطلب الوارد وعلى مسار القاعدة قبل أي مقارنة.

---

## 6. تطبيق حالة "معطّل" على مستوى النظام

### 6.1 المحرّك `pagecontrol.Engine`

مطابق بنيويًا لـ `features.Engine`:

- `Init(ctx, db, log) (*Engine, error)` — تحميل أولي + إعادة تحميل خلفية كل **20s** + `LISTEN pagecontrol_changed`
  (عند إشعار: إعادة تحميل فورية). فشل التحميل الأولي ⇒ **Fail‑open** (لقطة فارغة = لا حجب) + Warn صريح.
- لقطة غير قابلة للتغيير خلف `sync.RWMutex`:
  - `byExact map[string]Rule`
  - `prefixes []Rule` مرتّبة تنازليًا بطول `path` (الأطول أولًا).
- `Decision(path string) (blocked bool, rule Rule)`:
  1. `path = NormalizePath(path)`.
  2. جمع كل القواعد المطابقة: تطابق `exact` إن `path == rule.path`؛ تطابق `prefix` إن
     `path == rule.path || strings.HasPrefix(path, rule.path+"/")`.
  3. **الأكثر تحديدًا يفوز** (أطول `rule.path`؛ عند التساوي `exact` يتقدّم على `prefix`).
  4. النتيجة = `!winner.is_enabled`. لا قاعدة مطابقة ⇒ غير محجوب.
  - النتيجة: يمكن تعطيل `/vendor` (prefix) ثم استثناء `/vendor/orders` (exact مُفعّل) — التحديد الأطول يفوز.
- `Snapshot()` للواجهة الإدارية والاختبارات.

مُتاح Singleton عام (`pagecontrol.Blocked(path)`) لاستخدام اختياري في القوالب (إخفاء روابط لصفحات معطّلة — مرحلة 2).

### 6.2 نقطة الإنفاذ: تغليف الراوتر بالكامل

في `newRouter`، بدل `return r`:

```go
return pagecontrol.Guard(r, engine, log)
```

`Guard` هو `http.Handler` يلفّ mux الجذر:

1. `p := pagecontrol.NormalizePath(req.URL.Path)`.
2. إن `p` ضمن **قائمة التجاوز المُصرَّفة في الكود** (§6.3) ⇒ تمرير مباشر.
3. `blocked, rule := engine.Decision(p)`.
4. إن `blocked`:
   - سجّل `WarnContext` (`reason=page_disabled`, `rule_id`, `path`, `method`, `ip`) — بلا رد يكشف السبب.
   - إن `strings.HasPrefix(p, "/api/")` أو `Accept: application/json` ⇒ نفس مغلّف JSON‑404 المستخدم في
     `r.NotFound` (`apperr.KindNotFound`, `route.not_found`).
   - وإلا ⇒ نفس صفحة HTML‑404 القياسية (`authctx.notFoundHTML` أو صفحة 404 الموحّدة).
   - **الرد مطابق تمامًا لـ 404 الحقيقي** — لا فرق يُميّز "غير موجود" عن "معطّل".
5. وإلا ⇒ `r.ServeHTTP(w, req)`.

لماذا التغليف الخارجي لا `r.Use`: يسبق كل شيء (بما فيها المصادقة، CSRF، الجمهور)، يعمل لكل المسارات
(عامة/إدارية/API) بلا استثناء، ولا يتأثر بترتيب `Use` مع مسارات `/health` المُسجَّلة مبكرًا على `r`.

### 6.3 قائمة التجاوز المُصرَّفة (Hardcoded bypass) — دفاع في العمق

ثابتة في `pagecontrol/guard.go`، لا تُقرأ من القاعدة، فحتى صف قاعدة سيّئ لا يقفل النظام:

| المسار | الوضع |
|---|---|
| `/health`, `/ready`, `/api/v1/status` | prefix/exact |
| `/static/`, `/uploads/` (بادئات الأصول) | prefix |
| `/auth/` (تسجيل الدخول/الخروج/الاستعادة) | prefix |
| `/admin/system-pages` | prefix — لوحة التحكم نفسها |
| `/admin/dashboard`, `/admin/roles` | prefix — مخرج الطوارئ |
| `/lang/`, `/onboarding/pending` | prefix — استرداد الحساب |

هذه المسارات أيضًا تُبذَر في القاعدة بـ `is_system=true, is_lockable=false` (§7 حماية على طبقتين).

### 6.4 تمرير المحرّك

يُبنى المحرّك في `run()` (مثل `features.Init`) ويُمرَّر إلى `newRouter` كوسيط جديد،
أو يُستخدم Singleton عام كما تفعل `features`. مُفضّل التمرير الصريح (اختبارية أفضل).

---

## 7. المسارات المحميّة ومنع قفل الأدمن

طبقتان مستقلّتان:

1. **طبقة البيانات**: صفوف مبذورة `is_lockable=false`؛ خدمة `Toggle` ترفض `is_enabled=false` عليها
   برسالة i18n واضحة. كذلك تُرفض عملية تعطيل صف يطابق **مسار الطلب الحالي للأدمن** (تحذير "أنت على هذه الصفحة").
2. **طبقة الإنفاذ**: قائمة `guard.go` المُصرَّفة (§6.3) — تتجاوز حتى لو انقلب `is_lockable` بأي وسيلة.

بذور الهجرة 156:

```sql
INSERT INTO platform_admin.managed_pages (resource, path, match_mode, label, is_enabled, is_system, is_lockable, source)
VALUES
 ('admin','/admin/system-pages','prefix','{"ar":"التحكم في صفحات النظام","en":"System pages"}', true,true,false,'manual'),
 ('admin','/admin/dashboard','prefix','{"ar":"لوحة الإدارة","en":"Admin dashboard"}',           true,true,false,'manual'),
 ('admin','/admin/roles','prefix','{"ar":"الأدوار والصلاحيات","en":"Roles"}',                    true,true,false,'manual'),
 ('independent','/auth','prefix','{"ar":"الدخول والخروج","en":"Authentication"}',                true,true,false,'manual'),
 ('independent','/health','prefix','{"ar":"فحوص الصحة","en":"Health"}',                          true,true,false,'manual');
```

---

## 8. منطق الـ Middleware / Authorization

| الطبقة | المكوّن | الدور | الرد عند الرفض |
|---|---|---|---|
| 0 — خارج mux | `pagecontrol.Guard` | حجب Route معطّل لكل المستخدمين | 404 مطابق للحقيقي |
| 1 | `httpx.*` القائمة | RequestID/Recover/Logger/Security/Locale/Compress | — |
| 2 | `identityHttp.RequireAuth` / `OptionalAuth` | بناء `authctx.Actor` من الجلسة | 302 /auth/login |
| 3 | `authctx.RequireStaff` / `RequireCustomer` / `RequireVendor` | بوابة الجمهور | 404 |
| 4 | `authctx.RequirePagePermission("platform.page_control.view")` | بوابة صفحة `/admin/system-pages` | 404 |
| 5 | بوابات الكتابة `…create/update/delete` على مسارات POST | | 404 |

نقاط حَرِجة:
- Guard **لا يعيد توجيهًا ولا يعدّل السياق**؛ فقط يمرّر أو يرجع 404 → صفر تفاعل مع المصادقة/التفويض/الجلسة.
- Guard قبل `RequireAuth` → صفحة معطّلة تُخفى عن الزائر غير المسجّل أيضًا (متطلّب "جميع أنواع المستخدمين").
- بوابات الكتابة تُسجَّل في `admin_routes_*` كمجموعات `r.Group` منفصلة (نفس نمط الملف الحالي)، فتلتقطها
  الاختبارات الحارسة تلقائيًا.

---

## 9. Route Roots والمسارات المتداخلة (Nested)

- **الإنفاذ يقارن مسار الطلب الحقيقي (concrete) لا أنماط chi** — أبسط وأمتن ولا يتأثّر بتغيّر الأنماط.
- `match_mode='prefix'` يغطّي الجذر وكل ما تحته (`/admin/users` ⇒ يشمل `/admin/users/42/edit`).
- `match_mode='exact'` للصفحة الواحدة (`/about`، `/`).
- **قاعدة الأسبقية**: أطول `path` مطابق يفوز؛ عند التساوي `exact` قبل `prefix`. فالأدمن يعطّل فرعًا كاملًا
  ثم يُبقي صفحة واحدة منه بإضافة صف `exact` مُفعّل أطول.
- الاكتشاف يقترح تلقائيًا مسار الجذر الساكن للأنماط ذات البارامترات، فلا يحتاج الأدمن إدخال `{id}`.
- خارج النطاق v1: تعطيل حسب التابع (GET/POST) — الصفحة = كل توابع مسارها. مُوثَّق كتحسين لاحق.

---

## 10. ضمان عدم التعارض مع المسارات الحالية

| الخطر | الضمان |
|---|---|
| Route Conflicts | Guard لا يُسجّل أي مسار في chi؛ صفر تصادم توجيه. الأنماط المُكتشفة للعرض فقط. |
| Permission Conflicts | 4 مفاتيح جديدة بأسماء `platform.page_control.*` غير مستخدمة؛ فحص التكرار في `rbac.Build` يمنع التصادم. |
| Broken Existing Pages | الجدول يبدأ فارغًا (عدا صفوف محميّة مُفعّلة) ⇒ `Decision` يرجع "غير محجوب" دائمًا ⇒ سلوك مطابق للحالي بايت‑ببايت حتى أول Toggle. |
| Unexpected Redirects | Guard لا يعيد توجيهًا إطلاقًا؛ إما تمرير أو 404. |
| Auth/Authz issues | Guard بلا حالة، لا يلمس السياق ولا الجلسة ولا الكوكيز. |
| Self‑lockout | طبقتا حماية (§7). |
| Info leak | 404 مطابق حرفيًا + لا رأس/جسم مميّز + سجل خادمي فقط. |
| Cross‑instance staleness | عدّاد نسخة + `pg_notify` + polling 20s (شبكة أمان). |
| Infra failure at boot | Fail‑open + Warn (يوافق فلسفة `features` وقاعدة AGENTS.md R3: المنصة تستمر رغم تدهور البنية). |

---

## 11. واجهة الأدمن المطلوبة

### 11.1 المسارات (`internal/ui/admin_routes_platform.go` أو ملف جديد `admin_routes_pagecontrol.go`)

```go
r.Group(func(g chi.Router) {
    g.Use(authctx.RequirePagePermission("platform.page_control.view"))
    g.Get("/admin/system-pages", h.AdminSystemPagesPage)
})
r.Group(func(g chi.Router) {
    g.Use(authctx.RequirePagePermission("platform.page_control.update"))
    g.Post("/admin/system-pages/{id}/toggle", h.AdminSystemPageToggleSubmit)
    g.Post("/admin/system-pages/rescan",      h.AdminSystemPageRescanSubmit)
})
r.Group(func(g chi.Router) {
    g.Use(authctx.RequirePagePermission("platform.page_control.create"))
    g.Post("/admin/system-pages", h.AdminSystemPageCreateSubmit)
})
r.Group(func(g chi.Router) {
    g.Use(authctx.RequirePagePermission("platform.page_control.delete"))
    g.Post("/admin/system-pages/{id}/delete", h.AdminSystemPageDeleteSubmit)
})
```

### 11.2 الصفحة `internal/ui/pages/admin_page_control.templ`

- تبويبات/فلاتر حسب Resource: الكل / Admin / Vendor / Client / Independent.
- جدول: التسمية · المسار (mono) · نمط المطابقة (exact/prefix) · Resource · **مفتاح الحالة** (مُفعّل/معطّل) ·
  المصدر (مُكتشف/يدوي) · آخر تعديل (من/متى) · إجراءات (تعديل التسمية/الوصف، حذف إن يدوي وغير محمي).
- نموذج "إضافة صفحة": مسار (تحقّق فوري)، نمط، Resource (مقترَح)، تسمية ar/en، وصف.
- زر "إعادة فحص المسارات" → `rescan` (يضيف المُكتشف الجديد كـ مُفعّل).
- عند تعطيل صف `prefix`: نافذة تأكيد تعرض عدد الأنماط المشمولة (من `route_patterns[]`) وتحذيرًا.
- رفض الخادم لتعطيل صف محمي/الصفحة الحالية مع إشعار i18n.
- SSR + htmx، متسق مع بقية اللوحة. القالب بلا `<dialog>` خام (سقف 0) — استخدام `components.Modal`.

### 11.3 المعالِجات `internal/ui/admin_pagecontrol_handlers.go` (+ `_split2` عند تجاوز 400 سطر)

تتبع نمط `admin_settings_handlers.go`: `h.renderPage`, `h.redirectWithNotice`, `authctx.FromContext`,
`langOf(r)`, ولا نصوص عربية Hard‑coded — كل النصوص مفاتيح في `internal/shared/i18n` (ملف موجة جديدة، مثلاً
`catalog_admin_pagecontrol.go`).

---

## 12. قواعد التحقق (Validation) والأمان

### التحقق (خدمة `pagecontrol`، ليست الواجهة فقط)

- `path`: مُطبَّع، يطابق `^/[^?#\s]*$`، طول ≤ 512، فريد بين الصفوف غير المحذوفة.
- `match_mode ∈ {exact, prefix}`؛ يُمنع `prefix` + `/`.
- `resource ∈ {admin, vendor, client, independent}`؛ إن خالف بادئة المسار تحذير غير مانع.
- `label`: `ar` أو `en` واحدة على الأقل غير فارغة.
- Toggle: يُرفض على `is_lockable=false`؛ يُرفض على مسار يطابق مسار طلب الفاعل الحالي.
- Delete: فقط `source='manual'` و`is_system=false`.

### الأمان

- الإنفاذ Server‑side خارج mux، يسبق المصادقة → لا التفاف عبر URL مباشر، ولا عبر تبديل الدور، ولا عبر
  API مقابل HTML (كلاهما مُغطّى)، ولا عبر `/` نهائية أو حالة أحرف (تطبيع)، ولا عبر Query string (يُهمَل).
- 404 غير مُميِّز (لا تسريب معلومات).
- كتابات مُصرّح بها فقط؛ المفاتيح الخطيرة محجوبة من دور `admin` الأساسي.
- كل كتابة مُدقّقة (فاعل + قبل/بعد) في `platform.audit_log`.
- Fail‑open خيار توافر متعمّد وموثّق؛ البديل (Fail‑closed) يخاطر بانقطاع كلّي من فقدان كاش.
- لا سطح RLS جديد (الجدول عالمي؛ الوصول عبر `database.AsSystem` + بوابة صلاحية، مثل `feature_flags`).

---

## 13. الحالات الحدّية (Edge Cases)

| الحالة | المعالجة |
|---|---|
| `/` نهائية | تُطبَّع من الطلب والقاعدة قبل المقارنة (`/about/` = `/about`). |
| حالة الأحرف | مقارنة حسّاسة افتراضيًا؛ لا تحويل. |
| Query / Fragment | تُقصّ؛ المطابقة على المسار فقط. |
| الجذر `/` | يُدار بـ `exact` فقط؛ `prefix` عليه ممنوع بالتحقق. |
| بادئات متداخلة | الأطول يفوز؛ موثّق في الواجهة. |
| نمط `{id}` | يُخزَّن مسار الجذر الساكن للتفعيل؛ النمط الخام للعرض. |
| Toggle متزامن | عدّاد نسخة + آخر كتابة تفوز؛ الواجهة تعرض `updated_at`. |
| المحرّك غير مُهيّأ (DB ساقطة) | Fail‑open، خدمة كل شيء، Warn. |
| صفحة معطّلة مفتوحة في تبويب مستخدم | الطلب التالي 404 فورًا (خادمي، فور إعادة التحميل/الإشعار). |
| API يغذّي صفحة HTML معطّلة | v1: تعطيل مسار HTML لا يعطّل مسار XHR؛ الأدمن يضيف بادئة الـ API أيضًا. موثّق؛ ربط تلقائي مرحلة 2. |
| صفحة عامة مُعلَمة بـ Feature‑flag (`/offers`) | البوابتان تعملان؛ أيّهما يرجع 404. لا تعارض. |
| نمط مُكتشف اختفى | الصف يبقى (لا حذف تلقائي)؛ يُعلَّم في الواجهة "غير مُكتشف حاليًا". |
| صف مُكتشف عطّله الأدمن ثم rescan | `SyncDiscovered` لا يلمس `is_enabled` لصف موجود → يبقى معطّلًا. |
| تعطيل `/admin` كله (prefix) | مسموح، لكن `guard.go` يتجاوز `/admin/system-pages` و`/admin/dashboard` و`/admin/roles` → الأدمن يستعيد. |

---

## 14. الهجرة والتوافق الخلفي (Backward Compatibility)

### الهجرة `156_admin_page_control`

- **up**: إنشاء `managed_pages` + الفهارس + `page_control_version` + `bump_page_control_version()` + بذر الصفوف المحميّة.
- **down**: `DROP TABLE managed_pages; DROP FUNCTION bump_page_control_version; DROP TABLE page_control_version;`.
- لا Seeding لقائمة المسارات (الاكتشاف يفعل ذلك عند الإقلاع).
- توافق مع `schema_consistency_test.go`: قائمة الأعمدة في مستودع Go تطابق الهجرة حرفيًا.

### خطة الطرح (Rollout)

1. نشر الهجرة 156 (جدول + صفوف محميّة فقط).
2. نشر التطبيق بالمحرّك + Guard في وضع Fail‑open (كل شيء مُفعّل).
3. الاكتشاف عند الإقلاع يملأ `managed_pages` بكل مجموعات المسارات المعروفة `is_enabled=true`.
4. `rbac.Sync` ينشر المفاتيح الجديدة؛ `super_admin` يحصل عليها تلقائيًا؛ تُمنح للبقية حسب الحاجة.
5. الأدمن يفتح `/admin/system-pages` ويبدأ التعطيل الانتقائي.

**لا تغيير في أي سلوك واجهة موجود** حتى أول Toggle. الميزة قابلة للعكس بالكامل (down migration + خاملة ما دام الكل مُفعّلًا).

---

## 15. طريقة اختبار الميزة بالكامل

### 15.1 اختبارات وحدة — `internal/platform/pagecontrol/`

- `NormalizePath`: بادئة/لاحقة `/`، `//`، Query/Fragment، رفض غير الصالح.
- `ClassifyResource`: من البادئة.
- `Decision` — الأسبقية: `exact` مقابل `prefix`، الأطول يفوز، استثناء `exact` مُفعّل تحت `prefix` معطّل، لا تطابق ⇒ ممرّر.
- رفض المسارات المحميّة على مستوى الخدمة.
- `DiscoverRoutes` على راوتر لعبة ⇒ المرشّحون المتوقّعون؛ `SyncDiscovered` idempotent ولا يقلب Toggle الأدمن.

### 15.2 اختبارات تكامل — `pagecontrol/guard_test.go` (httptest)

- مسار `exact` معطّل ⇒ 404 لـ: زائر / عميل / مورّد / أدمن (مصفوفة جمهور).
- مُفعّل ⇒ يمرّر.
- `prefix` معطّل ⇒ كل الأبناء 404؛ ابن `exact` مُفعّل تحته ⇒ 200.
- مسار محمي لا يُحجب أبدًا ولو قال الصف `is_enabled=false`.
- `/health` لا يُحجب أبدًا.
- جسم 404 مطابق حرفيًا لـ 404 الحقيقي (HTML وJSON) — لا تسريب.
- Guard لا يعيد توجيهًا ولا يضيف رؤوسًا.

### 15.3 اختبارات الحُرّاس القائمة (يجب أن تمرّ دون تعديل يدوي)

- `test/rbac_guard_test.go`: المفاتيح `platform.page_control.*` معلَنة؛ عنصر القائمة يحرس مسارًا؛ كل بوابة تسمّي مفتاحًا معلَنًا.
- `test/route_audience_test.go`: `/admin/system-pages` داخل `RequireStaff` + `RequirePagePermission`.
- `internal/platform/rbac/rbac_test.go`: `TestCatalogBuilds`, `TestActionsImplyTheirPage`, `TestOwnerHoldsEverythingInScope`.

### 15.4 إبطال عبر العمليات

محرّكان فوق نفس القاعدة؛ أحدهما يكتب ⇒ الآخر يرى التغيير بعد إشعار/إعادة تحميل خلال < 20s (وفوريًا مع `pg_notify`).

### 15.5 الهجرة

- `migrate` ثم `migrate-status` نظيف؛ `down` يعيد المخطط تمامًا.
- إعادة تشغيل التطبيق ⇒ الاكتشاف idempotent (لا صفوف مكرّرة، لا Toggle مقلوب).

### 15.6 انحدار شامل

`make check` كامل: `check-file-size` (≤400)، `check-hardcoded-arabic` (0)، `check-no-cdn`،
توليد templ، `deadcode` ratchet، `go test ./...`، `golangci-lint`.

### 15.7 يدوي (قبل الاعتماد)

1. تعطيل `/about` ⇒ 404 كزائر + كأدمن؛ الرابط في الفوتر يبقى (مرحلة 2 لإخفائه) لكن النقر ⇒ 404.
2. تعطيل `/vendor/orders` (exact) ⇒ المورّد يفقد الصفحة، بقية `/vendor/*` تعمل.
3. تعطيل `/admin` (prefix) ⇒ `/admin/system-pages` و`/admin/dashboard` تبقى ⇒ إعادة التفعيل.
4. محاولة تعطيل `/admin/system-pages` ⇒ رفض بإشعار.
5. إضافة `/coming-soon` يدويًا قبل وجود Route ⇒ يُحفظ؛ لا أثر حتى يُنشأ الـ Route.
6. `rescan` بعد إضافة Route جديد في الكود ⇒ يظهر كصف مُكتشف مُفعّل.

---

## 16. ملفات ستُنشأ/تُعدَّل

**جديدة**
- `db/migrations/156_admin_page_control.up.sql` / `.down.sql`
- `internal/platform/pagecontrol/domain.go` · `engine.go` · `store.go` · `discovery.go` · `guard.go` · `pagecontrol_test.go` · `guard_test.go`
- `internal/ui/admin_pagecontrol_handlers.go` (+ `_split2` عند اللزوم)
- `internal/ui/admin_routes_pagecontrol.go`
- `internal/ui/pages/admin_page_control.templ`
- `internal/shared/i18n/catalog_admin_pagecontrol.go`

**معدَّلة**
- `internal/platform/rbac/catalog_admin.go` (4 مفاتيح) · `nav_admin.go` (عنصر قائمة) · `roles.go` (`withheld`)
- `cmd/server/main.go` (`pagecontrol.Init` في `run`) · `cmd/server/routes.go` أو `main.go` (`Guard` حول الراوتر + `SyncDiscovered`)
- `internal/ui/handlers.go` (`registerAdminPagecontrolRoutes` ضمن `RegisterAdminRoutes`)
- `internal/ui/handler_deps.go` (تمرير `*pagecontrol.Engine` إن لزم للواجهة)

---

## 17. المخاطر والمقايضات

| القرار | البديل | لماذا هذا الخيار |
|---|---|---|
| Fail‑open عند فشل التحميل | Fail‑closed | تجنّب انقطاع كلّي من فقدان كاش؛ يوافق `features` وAGENTS.md R3 |
| مطابقة المسار الحقيقي لا أنماط chi | مطابقة الأنماط عبر `RouteContext` | أمتن، لا يتأثر بتغيّر الأنماط، أبسط منطقًا؛ الأنماط للعرض فقط |
| لا Seeding للمسارات في الهجرة | بذر كامل | مصدرَان يتباعدان = العطل المقصود تفاديه (تعليق الهجرة 145) |
| تغليف خارجي للراوتر | `r.Use` عام | يسبق كل شيء، يغطّي `/health` بلا ترتيب هشّ |
| 4 مفاتيح صلاحية للميزة | مفتاح لكل صفحة | يمنع انفجار الكتالوج وتعارض اختبارات الحُرّاس |
| حجب `.create/.update/.delete` من دور admin | منحها | ميزة قوية/خطيرة؛ حصر بـ super_admin افتراضيًا |
