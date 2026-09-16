# PHASE 5 — The Admin Panel

**Depends on:** Phase 0 (permission gates — every route here needs one),
Phase 1 (institutional works), Phase 4 (admin import).
**Blocks:** Phase 10 verification.
**Tasks:** 10 areas, ~24 screen groups.
**Size:** the largest surface gap. Laravel has 183 admin routes; Go has 74.

## Why this phase exists

Go's admin sidebar has 13 entries. Laravel's has ~50 across 10 groups. Twenty-four
distinct admin capabilities are entirely absent, including **the ability to edit
a role** — there is currently no way to change a permission through the product.

---

## 5.1 Ground rules for this phase

1. **Every route gets `RequirePagePermission`** (built in Phase 0 Task 0.2).
   Read gates GET, write gates POST. No exceptions except `/admin/dashboard`.
2. **Every sidebar entry is permission-aware** — a staff member must not see a
   link that 404s for them.
3. **`AsSystem` everywhere, with a comment.** Admin screens read across tenants
   by definition (rule R4). Each call gets a one-line justification.
4. **Reuse `@components.DataTable`.** Twenty-four listing screens must not
   produce twenty-four bespoke tables.
5. **Every listing reproduces Laravel's filters, sort, page size and actions.**
   Read the Livewire component's `$filters`, `$sortBy`, `->paginate(N)` and the
   Blade view's buttons before building.
6. **File size (rule R6).** `RegisterAdminRoutes` is already being split in
   Phase 0. Continue: one `internal/ui/admin_routes_<area>.go` and one
   `internal/ui/admin_<area>_handlers.go` per group below.

---

## 5.2 The authoritative admin sidebar

Extracted from `Laravel/resources/views/components/layouts/admin.blade.php`.
**Reproduce these 10 groups, in this order, with these Arabic labels.**

### Group 1 — الرئيسية والمتابعة (`dashboard_and_overview`)
| Laravel | Go | Status |
|---|---|---|
| `/admin/dashboard` — الرئيسية | `/admin/dashboard` | ✅ |
| `/admin/activities` — السجل والنشاطات | `/admin/audit` | ⚠️ unlinked (Phase 0 Task 0.5.2) |
| `/admin/notifications` — الإشعارات | `/admin/notifications` | ❌ Task 5.9 |

### Group 2 — المؤسسة والمشرفين (`organization_management`)
| Laravel | Go | Status |
|---|---|---|
| `/admin/organization` (+ `/{id}`, `/{id}/info`, `/{id}/users`, `/{id}/branches`) | `/admin/organizations` | ⚠️ index only — detail screens missing |
| `/admin/orgniazions/products-import` | `/admin/organizations/imports` | ❌ Phase 4 Task 4.3 |
| `/admin/chat/tree` | — | ❌ Task 5.7 |
| `/admin/chat/history` (+ `/{id}`) | — | ❌ Task 5.7 |
| `/admin/branches` (+ `/{id}`, `/{id}/products`, `/{id}/users`) | — | ❌ Task 5.2 |
| `/admin/admins` (+ create, `{id}/edit`) | — | ❌ Task 5.3 |
| `/admin/users` (+ `{id}`, `{id}/info`) | `/admin/users` | ⚠️ index only |
| `/admin/full-user` (+ new-clients, new-clients/{id}) | — | ❌ Task 5.3 |
| `/admin/customer-list` (+ create, `{id}`, `{id}/edit`, `{id}/info`) | — | ❌ Task 5.3 |
| `/admin/vendor-list` (× 5) | — | ❌ Task 5.3 |
| `/admin/admin-list` (× 6, incl. create-fast) | — | ❌ Task 5.3 |
| `/admin/employee-activities` | — | ❌ Task 5.3 |
| `/admin/roles` | — | ❌ Task 5.4 |
| `/admin/admin-roles` (+ `{id}`) | — | ❌ Task 5.4 |
| `/admin/admin-permissions` | — | ❌ Task 5.4 |
| `/admin/user-address` (+ `{id}`) | — | ❌ Task 5.3 |
| `/admin/user-organization` | — | ❌ Task 5.3 |
| `/admin/want-delete` (+ `{id}`) | POST handlers only | ⚠️ Task 5.3 |
| `/admin/ask-for` (+ `{id}`) | — | ❌ Task 5.8 |
| `/admin/documents/{id?}` | `/admin/documents` | ✅ |

### Group 3 — الإعدادات والمحتوى (`settings_and_content`)
| Laravel | Go | Status |
|---|---|---|
| `/admin/settings` | `/admin/settings` | ✅ |
| `/admin/full-powers` | `/admin/settings` (features tab) | ✅ |
| `/admin/translations` | `/admin/translations` | ⚠️ unlinked |
| `/admin/policies` | `/admin/policies` | ⚠️ unlinked |
| `/admin/social-media` | — | ❌ Task 5.6 |
| `/admin/highlight-sections` | — | ❌ Task 5.6 |
| `/admin/institutional-work` | `/admin/institutional` | ✅ |
| `/admin/cities` | `/admin/cities` | ✅ |
| `/admin/countries` | — | ❌ Task 5.6 |
| `/admin/user-address` | — | ❌ Task 5.3 |

### Group 4 — إدارة المنتجات والمخزون (`inventory_management`)
| Laravel | Go | Status |
|---|---|---|
| `/admin/products` (+ `{id}`) | `/admin/products` | ⚠️ index only |
| `/admin/product-child` (+ `{id}`) — المنتجات الفرعية | — | ❌ Task 5.5 |
| `/admin/adv-products` | — | ❌ Task 5.5 |
| `/admin/apis-products` | — | ❌ Task 5.5 |
| `/admin/categories` | — | ❌ Task 5.6 |
| `/admin/brands` | — | ❌ Task 5.6 |
| `/admin/import-products` — استيراد المنتجات | Phase 4 | ⚠️ |
| `/admin/image-products` — استيراد المنتجات image | Phase 4 | ❌ |
| `/admin/saveing-products` + `/saving-products/user/{id}` + `/org/{id}` + `/products-saving/import` + org/customer upload — منتجات التوفير | — | ❌ Task 5.5 |
| `/admin/stocks` | — | ❌ Task 5.5 |
| `/admin/warehouses` (+ `{id}`) | — | ❌ Task 5.5 |
| `/admin/user/temparte-warehouses` (+ `{id}`) — مستودعات المؤقتة | — | ❌ Task 5.5 |
| `/admin/my/temparte-warehouses` — مستودعاتي المرفوعة | — | ❌ Task 5.5 |
| `/admin/import/temparte-warehouses` — رفع مستودع مؤقت | — | ❌ Task 5.5 |
| `/admin/admins/temparte-warehouses` — مستودعات كافة المشرفين | — | ❌ Task 5.5 |
| `/admin/plan/temparte-warehouses` (+ `{id}`, `/request`, `/request/{id}`) — باقات المستودعات | — | ❌ Task 5.5 |
| `/admin/user-plan/temparte-warehouses` — اشتراكات المستخدمين | — | ❌ Task 5.5 |
| `/admin/upload-warehouse-file` (POST) | — | ❌ Task 5.5 |
| `/admin/compare-discounts/upload` — global discounts | Phase 2 | ❌ |

### Group 5 — المبيعات والمالية (`sales_and_finance`)
| Laravel | Go | Status |
|---|---|---|
| `/admin/orders` (+ `{id}`) | `/admin/orders` | ⚠️ index only |
| `/admin/orders/offers` (+ `{id}`) — طلبات العروض | — | ❌ Task 5.10 |
| `/admin/earnings/order`, `/admin/earnings/offers` | — | ❌ Task 5.10 |
| `/admin/invoices` | — | ❌ Task 5.10 |
| `/admin/payments` | — | ❌ Task 5.10 |
| `/admin/wallets` — المحافظ | — | ❌ Task 5.10 |

### Group 6 — العروض والتسويق (`offers_promotions_management`)
| Laravel | Go | Status |
|---|---|---|
| `/admin/offers` (+ create, `{id}`, `{id}/edit`, `{id}/locations`, `/locations`) | `/admin/offers` | ⚠️ index + status only |
| `/admin/offers-packages` — لوحة العروض والرعايات | — | ❌ Phase 8 |
| `/admin/offers-packages/packages` (+ `{id}`) — باقات الرعاية والإعلان | — | ❌ Phase 8 |
| `/admin/offer-sponsorships` (+ `{id}`) — اشتراكات ورعايات العروض | — | ❌ Phase 8 |
| `/admin/offers-packages/promotions` (+ `{id}`) — الحملات الترويجية | — | ❌ Phase 8 |
| `/admin/offers-packages/views` (+ `{id}`) — إحصائيات المشاهدات | — | ❌ Phase 8 |
| `/admin/offers-packages/clicks` (+ `{id}`) — إحصائيات النقرات | — | ❌ Phase 8 |
| `/admin/ad-plan` (+ create, `{id}`, `{id}/edit`) | — | ❌ Phase 8 |
| `/admin/ads` (+ create, `{id}`, `{id}/edit`, `{id}/action`) — إدارة الإعلانات | — | ❌ Phase 8 |

### Group 7 — أدوات أخرى (`tools`)
| Laravel | Go | Status |
|---|---|---|
| `/admin/contact-us` (+ `{id}`) — رسائل التواصل | `/admin/messages` | ⚠️ unlinked, verify it is the same thing |
| `/admin/plans`, `/admin/plans-info`, `/admin/plan-types`, `/admin/plan-features`, `/admin/plans/subscriptions` — خطط الاشتراك | `/admin/plans` | ⚠️ one screen for five |
| `/admin/api-integrations` | — | ❌ Task 5.6 |
| `/admin/jobs` (+ `{id}/applications`) | `/admin/jobs` | ⚠️ unlinked, applications missing |
| `/admin/weekly-coverages` (+ add, edit/{id}, `{id}`) | — | ❌ Task 5.2 |
| `/admin/session-plan` (+ create, `{id}`, `{id}/edit`, `/requests`, `/requests/{id}`) | — | ❌ Phase 9 |
| `/admin/first-look` | — | ❌ Task 5.9 |

### Group 8 — إدارة الحذف والمحذوفات (`system_deletes_trash`)
| Laravel | Go | Status |
|---|---|---|
| `/admin/deletes-lists` (+ `{model}`, `{model}/{id}`) — قائمة الحذف الشاملة | — | ❌ Task 5.11 |
| `/admin/trash-list` (+ `{model}`, `{model}/{id}`) — سلة المهملات | — | ❌ Task 5.11 |

### Group 9 — مطور (`developer`)
| Laravel | Go | Status |
|---|---|---|
| `/admin/developer/sql-console` | `/admin/developers` | ✅ (hardened in Phase 0) |
| `/admin/ai/test` | `/admin/developers` (AI tab) | ✅ |

### Group 10 — سجلات المراقبة والتدقيق (`monitoring_and_audit_logs`)
| Laravel | Go | Status |
|---|---|---|
| `/admin/full-activity-logs` (+ `{id}`) — سجلات الأنشطة والتدقيق | `/admin/audit` | ⚠️ no detail view |
| `/admin/full-error-logs` (+ `{id}`) — سجلات الأخطاء والتشخيص | service exists, no page | ❌ Task 5.9 |
| `/admin/full/admin-notification` (+ `{id}`) | — | ❌ Task 5.9 |
| `/admin/system-page` (+ `{system}`) | table exists, no page | ❌ Task 5.9 |

---

## TASK 5.1 — Rebuild the admin shell

Before adding screens, make the shell able to hold them.

- `internal/ui/layouts/admin.templ`: replace the flat 13-link list with the 10
  groups above, using collapsible sections (`nav-sep` in Laravel).
- Each link wrapped in its permission check.
- Active-state highlighting matching Laravel's `request()->is(...)` patterns,
  including wildcards (`admin/products*`).
- The layout will exceed 400 lines — that limit applies to **Go files**; `.templ`
  files generate Go, so check whether `make check-file-size` covers them. If it
  does, split the nav into `internal/ui/layouts/admin_nav.templ`.
- Search/command palette over admin screens (`@components.CommandPalette` exists).

**Completion:** every group renders; a `support` actor sees only their permitted
subset; no visible link 404s.

---

## TASK 5.2 — Organizations, branches, coverage oversight

### Screens
| Route | Laravel component | Contents |
|---|---|---|
| `/admin/organizations` | `Admin/Organization` | ✅ exists — verify filters match |
| `/admin/organizations/{id}` | `Organizations/ShowList` | full org profile |
| `/admin/organizations/{id}/info` | `Organizations/Info` | registration data + documents |
| `/admin/organizations/{id}/users` | `Organizations/UsersList` | members with roles |
| `/admin/organizations/{id}/branches` | `Organizations/BranchesList` | branches + map |
| `/admin/branches` | `Admin/Branches` | all branches, all orgs |
| `/admin/branches/{id}` | `Admin/BranchShow` | branch detail |
| `/admin/branches/{id}/products` | `Admin/BranchProducts` | that branch's catalog |
| `/admin/branches/{id}/users` | `Admin/BranchUsers` | that branch's staff |
| `/admin/weekly-coverages` | `AdminWeeklyCoverage` | all coverage, all vendors |
| `/admin/weekly-coverages/add` | `AdminAddWeeklyCoverage` | create |
| `/admin/weekly-coverages/edit/{id}` | `AdminAddWeeklyCoverage` | edit (same component) |
| `/admin/weekly-coverages/{id}` | `AdminShowWeeklyCoverage` | detail + map |

Reuse the Phase 0 coverage forms; admin can act on any organization.

**Permissions:** `org.admin` / `org.branch.view` / `workflow.coverage.manage` —
map from Laravel's `organizations_view`, `organization_branches_view`,
`organization_users_view`, `orders_view` (Laravel gates coverage under
`orders_view`; **port that oddity** and note it).

---

## TASK 5.3 — The Full User panel

Laravel's largest admin area: 22 routes. Go has one flat `/admin/users`.

### Screens
| Route | Laravel component |
|---|---|
| `/admin/full-user` | `FullUser/Index` |
| `/admin/full-user/new-clients` | `FullUser/NewClientsList` |
| `/admin/full-user/new-clients/{id}` | `FullUser/NewClientsShow` |
| `/admin/customer-list` | `FullUser/CustomerList` |
| `/admin/customer-list/create` | `FullUser/CustomerCreate` |
| `/admin/customer-list/{id}` | `FullUser/CustomerShow` |
| `/admin/customer-list/{id}/edit` | `FullUser/CustomerEdit` |
| `/admin/customer-list/{id}/info` | `FullUser/CustomerInfo` |
| `/admin/vendor-list` (× 5, same shape) | `FullUser/Vendor*` |
| `/admin/admin-list` (× 5) + `/admin-list/create-fast` | `FullUser/Admin*`, `AdminCreateFast` |
| `/admin/admins`, `/admin/admins/create`, `/admin/admins/{id}/edit` | `Admins`, `AdminsCreate`, `AdminsEdit` |
| `/admin/users`, `/admin/users/{id}`, `/admin/users/{id}/info` | `Users`, `UserDetails`, `Users/Info` |
| `/admin/employee-activities` | `FullUser/EmployeeActivities` |
| `/admin/user-address`, `/admin/user-address/{id}` | `UserAddress/Index`, `Show` |
| `/admin/user-organization` | `UserOrganization` |
| `/admin/want-delete`, `/admin/want-delete/{id}` | `WantDeleteIndex`, `WantDeleteShow` |

### Notes
- `FullUser/HasSessionPlanModal` is a shared modal — reproduce as a component.
- Creating a user from the admin panel is a **write path that must not bypass
  password rules** — reuse the identity service, never insert directly.
- `want-delete` POST handlers already exist
  (`AdminUserDeletionApproveSubmit` / `RejectSubmit`); this task adds the pages.
- `employee_activities` table is missing (audit §6.1). Add it:
  `db/migrations/090_employee_activities.up.sql`, populated by the equivalent of
  Laravel's `EmployeeActivityObserver` — in Go, a service-layer hook or a River
  job fed by domain events. **Inspect the observer to see exactly which actions
  are recorded.**

---

## TASK 5.4 — Roles & permissions editor

**There is currently no way to edit a role in the Go product.** This is the
highest-impact admin gap after Phase 0.

### Screens
| Route | Laravel |
|---|---|
| `/admin/roles` | `Admin/Roles` — supplier (organization) roles |
| `/admin/admin-roles` | `AdminRoles/Index` — platform staff roles |
| `/admin/admin-roles/{id}` | `AdminRoles/Show` — role detail + permission matrix |
| `/admin/admin-permissions` | `AdminPermissions/Index` — the permission catalogue |

### Requirements
- The permission matrix UI: permissions grouped by `module` (Laravel groups by
  `group`), checkboxes, save.
- Creating/editing a role writes `identity.roles` + `identity.role_permissions`
  (staff) or `org.roles` + `org.role_permissions` (organization roles).
- **System roles must not be editable into a broken state.** `super_admin` keeps
  every permission; a role cannot remove the last `super_admin`.
- Changing a role must invalidate affected users' cached permissions —
  check how `authctx.Actor.Permissions` is populated (`ResolveTenant`) and
  whether it is cached. If cached, bust it.
- Audit-log every permission change (who, what, before, after).

### Tests
- T5: editing roles requires `identity.role.manage`
- T1: a role edit cannot leave zero super admins
- T6: granting a permission takes effect on the target user's next request
- T17: **privilege escalation guard** — an `admin` cannot grant themselves
  `platform.developer.sql`. Decide the rule from Laravel (can an admin edit their
  own role?) and enforce it.

---

## TASK 5.5 — Catalog, inventory, warehouses, saving products

### 5.5a Products
| Route | Laravel |
|---|---|
| `/admin/products` | `Admin/Products` ✅ |
| `/admin/products/{id}` | `AdminProductShow` ❌ |
| `/admin/product-child` | `AdminProductChildern` ❌ |
| `/admin/product-child/{id}` | `AdminProductChildernShow` ❌ |
| `/admin/adv-products` | `AdvProducts` ❌ |
| `/admin/apis-products` | `ApisProducts` ❌ |

**Inspect what `adv-products` and `apis-products` actually are** — the names
suggest "advertised products" and "products from API integrations". Read the
components before building.

### 5.5b Inventory
| Route | Laravel |
|---|---|
| `/admin/stocks` | `EmployeeStocks` (reused by admin) |
| `/admin/warehouses` | `EmployeeWarehouses` |
| `/admin/warehouses/{id}` | `WarehouseDetails` |

### 5.5c Temp warehouses (مستودعات مؤقتة) — 14 routes

This is an entire subsystem with no Go equivalent beyond an empty
`inventory.temp_warehouses` table.

**Inspect first — this is complex:**
```bash
cat F:/Dawa\ 24/Laravel/app/Services/WarehouseLifecycleService.php
cat F:/Dawa\ 24/Laravel/app/Jobs/ProcessWarehouseBatch.php
cat F:/Dawa\ 24/Laravel/app/Jobs/ProcessWarehouseFile.php
cat F:/Dawa\ 24/Laravel/app/Models/UserTemparteWarehouse.php
cat F:/Dawa\ 24/Laravel/app/Models/PlanTemparteWarehouse.php
cat F:/Dawa\ 24/Laravel/app/Models/FatherUserTemparteWarehouse.php
cat F:/Dawa\ 24/Laravel/app/Models/UserPlanTemparteWarehouse.php
cat F:/Dawa\ 24/Laravel/app/Http/Controllers/Admin/WarehouseUploadController.php
```

Write `docs/modules/inventory-temp-warehouses.md` documenting the lifecycle
before coding. Missing tables (audit §6.1): `plan_temparte_warehouses`,
`user_plan_temparte_warehouses`, `father_user_temparte_warehouses`.

Screens: `/admin/user/temparte-warehouses` (+ `{id}`), `/admin/my/…`,
`/admin/import/…`, `/admin/admins/…`, `/admin/plan/…` (+ `{id}`, `/request`,
`/request/{id}`), `/admin/user-plan/…`, `POST /admin/upload-warehouse-file`.

### 5.5d Saving products (منتجات التوفير)

Table reinstated in Phase 0 Task 0.6.1.

| Route | Laravel |
|---|---|
| `/admin/saving-products` (and the `/saveing-products` misspelling as an alias) | `Admin/SavingProducts` |
| `/admin/saving-products/user/{userId}` | `SavingProductsUser` |
| `/admin/saving-products/org/{organizationId}` | `SavingProductsOrg` |
| `/admin/products-saving/import` | `SavingProducts/ImportLanding` |
| `/admin/organizations/{id}/saving-products/upload` | `Organizations/SavingProductsImport` |
| `/admin/customers/{id}/saving-products/upload` | `Customers/SavingProductsImport` |

> **Keep the `/saveing-products` misspelled alias.** Laravel registers both; old
> links exist. Register the correct spelling as canonical and 301 the misspelling.

---

## TASK 5.6 — Reference data & content

| Route | Laravel component | Table |
|---|---|---|
| `/admin/categories` | `Admin/Categories` | `catalog.categories` |
| `/admin/brands` | `Employee/Brands` | `catalog.brands` |
| `/admin/countries` | `CountriesCo` | `platform_admin.countries` |
| `/admin/cities` | `CitiesCo` | ✅ exists |
| `/admin/social-media` | `Admin/SocialMedia` | `org.organization_social_media` |
| `/admin/highlight-sections` | `Admin/HighlightSections` | `promo.highlight_sections` |
| `/admin/api-integrations` | `ApiIntegrations` | `platform_admin.api_integrations` |
| `/admin/translations` | `TranslationManager` | ✅ exists, unlinked |
| `/admin/policies` | `Admin/Policies` | ✅ exists, unlinked |

All simple CRUD. Use `@components.DataTable` + a shared edit-modal component.
Build **one** reusable admin-CRUD page template and parameterise it — nine
near-identical screens must not be nine hand-written files.

`/admin/api-integrations` handles credentials — **never render a stored secret
back to the browser.** Show a masked value and a "replace" action.

---

## TASK 5.7 — Chat tree & history

| Route | Laravel |
|---|---|
| `/admin/chat/tree` | `AdminChatTree` |
| `/admin/chat/history` | `AdminChatHistory` |
| `/admin/chat/history/{id}` | `AdminChatHistoryShow` |

**Inspect what "chat tree" is** — Laravel has `trees`, `tree_options`,
`tree_results` tables, which Go ported as `catalog.finder_questions` /
`finder_options` / `finder_results`, and `/admin/finder` already exists.
Determine whether `chat/tree` is the same thing under another name (a decision
tree driving an assistant) or a separate concept.

```bash
cat F:/Dawa\ 24/Laravel/app/Livewire/Admin/ChatTree.php
cat F:/Dawa\ 24/Laravel/app/Models/Tree.php
cat F:/Dawa\ 24/Laravel/docs/chat_ai_institutional_works_ar.md
```

Chat history reads `chat.conversations` / `chat.messages` (both exist in Go).
The admin view is oversight of customer↔vendor conversations —
**this is sensitive.** Gate it behind a dedicated permission, log every access to
the audit trail, and record the decision in `docs/modules/chat.md`.

---

## TASK 5.8 — AskFor (طلبات المستندات)

| Route | Laravel |
|---|---|
| `/admin/ask-for` | `AskForIndex` |
| `/admin/ask-for/{id}` | `AskForShow` |

An admin asks an organization for a document or an action; the org sees it in
their panel (the vendor layout references `$askFor->action_url`).

Table `ask_fors` has no Go equivalent — check whether `workflow.requests` fits
before adding a table. Record the decision.

Vendor/customer side: a banner or notification linking to the requested action.
The Laravel vendor layout shows it inline — reproduce that.

---

## TASK 5.9 — Monitoring, logs, notifications, system pages

### 5.9a Error logs
Service methods already exist (`ListErrorLogs`, `GetErrorLogByID`,
`UpdateErrorLogStatus`, `GetErrorDiagnosticsMetrics`) with **no page and, until
Phase 0 Task 0.3, no table**. Build:
- `/admin/full-error-logs` — filterable list (severity, status, date, route)
- `/admin/full-error-logs/{id}` — full detail: trace, request, response, context
- status transitions NEW → INVESTIGATING → RESOLVED → IGNORED
- **error capture**: something must write these rows. Add middleware that
  records unhandled errors and 5xx responses into `platform_admin.error_logs`.
  Inspect `FullErrorLogService.php` (692 lines) for what Laravel captures.

### 5.9b Activity logs
`/admin/full-activity-logs` + `/{id}`. `platform.audit_log` exists;
`/admin/audit` exists but has no detail view. Add it, plus filters matching
`FullActivityLogsIndex`.

### 5.9c Notifications
`/admin/notifications`, `/admin/full/admin-notification`, `+ /{id}`.
`notifications.admin_notifications` exists. Build the pages and the
"send a notification to a segment" action if Laravel has it.

### 5.9d System resources
`/admin/system-page`, `/admin/system-page/{system}`.
`platform_admin.system_resources` exists with no screen. Read
`SystemResources/Index`, `SystemResourcePolicy`, `SystemResourceStatus`,
`SystemResourceType`, `SystemType` enums, and the `system.resource` middleware —
this appears to be a feature-availability mechanism layered over `full_powers`.
Reproduce it, including the middleware behaviour.

### 5.9e First look
`/admin/first-look` — `first_admin_login` middleware forces a new admin through
it. Reproduce the middleware and the screen.

---

## TASK 5.10 — Finance

| Route | Laravel |
|---|---|
| `/admin/orders/offers` (+ `{id}`) | `AdminOfferOrders`, `ShowOfferOrder` |
| `/admin/earnings/order` | `EarningsOrder` |
| `/admin/earnings/offers` | `EarningsOffers` |
| `/admin/invoices` | `AdminInvoices` |
| `/admin/payments` | `AdminPayments` |
| `/admin/wallets` | `AdminWallets` |
| `/admin/plans-info`, `/plan-types`, `/plan-features`, `/plans/subscriptions` | `PlansInfoIndex`, `PlanTypesIndex`, `PlanFeaturesIndex`, `PlansSubscriptions` |

Missing tables (audit §6.1): `plan_types`, `subscription_histories`,
`subscription_users`, `user_plan_histories`. Add them.

**Every money figure uses `money.Amount` and is asserted exactly (T8).**
Earnings screens are the commission/revenue reports — read the components
carefully; the calculation is a business rule that must match Laravel to the
minor unit.

---

## TASK 5.11 — Deletes list & trash

`/what-in` lists this as a headline admin pillar: "Trash & Deleted Items
(استرجاع أو الحذف النهائي للبيانات مع التاريخ)".

| Route | Laravel |
|---|---|
| `/admin/deletes-lists` | `DeletesListsIndex` — every model with soft-deletable rows |
| `/admin/deletes-lists/{model}` | `DeletesListsModel` |
| `/admin/deletes-lists/{model}/{id}` | `DeletesListsShow` |
| `/admin/trash-list` | `TrashListIndex` |
| `/admin/trash-list/{model}` | `TrashListModel` |
| `/admin/trash-list/{model}/{id}` | `TrashListShow` |

**Inspect the difference between the two** — they look near-identical. Read both
components; one may be "pending deletion" and the other "already deleted".

### Implementation
- A **registry** of soft-deletable tables: schema, table, display name (ar/en),
  the permission required to restore, and the columns to show. Hand-maintained
  is acceptable; generated from `information_schema` for any table with
  `deleted_at` is better and will not drift.
- Restore = `UPDATE … SET deleted_at = NULL` with an audit row.
- Purge = hard `DELETE`. **This is destructive and irreversible.** It needs:
  its own permission, a typed confirmation, and an audit entry.
- **FK safety:** restoring a child whose parent is still deleted must be refused
  with a clear message, not a constraint error.

### Tests
- T5: restore and purge each require their own permission
- T1: restoring an orphan is refused
- T6: restore round-trip on ≥3 different tables
- T18: purge is logged with actor, table, row id, and the row's contents

---

## PHASE 5 COMPLETION GATE

```bash
make check
go run ./cmd/migratecheck -from 90 -roundtrip
go test ./test/... -run 'AdminGuard|Audience'
```

- [ ] All 10 sidebar groups render with Laravel's Arabic labels, in order
- [ ] Every admin route has a permission gate; the guard test walks the real router
- [ ] Sidebar links are permission-aware; nothing visible 404s
- [ ] **A role's permissions can be edited through the product** (Task 5.4)
- [ ] Trash/restore works on ≥3 tables with FK safety
- [ ] Error logs are captured and viewable
- [ ] Every listing reproduces Laravel's filters, sort, page size and actions
- [ ] No admin screen renders a stored secret
- [ ] `PROGRESS.md` updated for 5.1–5.11
