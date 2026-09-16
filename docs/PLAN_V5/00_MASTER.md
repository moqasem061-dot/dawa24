# PLAN V5 — Laravel → Go Completion Plan (MASTER)

**Audience:** Gemini 3.7 Flash (executing agent)
**Authority:** this plan supersedes `REBUILD_V2.md`, `AUDIT_V2.md`, `AUDIT_V3.md` where they conflict.
**Evidence base:** `docs/PARITY_AUDIT_V4.md` (read it before Phase 0).
**Reference system:** `F:\Dawa 24\Laravel` — the source of truth for *behaviour*.
**Target system:** `F:\Dawa 24\dawa24-store` — the source of truth for *architecture*.

---

## 0.1 Read this first — the one sentence that governs everything

> **Laravel decides WHAT the system does and HOW IT LOOKS. Go decides HOW IT IS BUILT.**

You are not designing a product. Every screen, every field, every button, every
Arabic label, every workflow step, every permission key already exists in the
Laravel project. Your job is to reproduce that surface on the Go architecture.

When those two collide, this table decides:

| Question | Authority |
|---|---|
| Which screens exist, and what is on them | **Laravel** |
| Sidebar order, grouping, icons, Arabic labels | **Laravel** |
| Business rules, calculations, status values, edge cases | **Laravel** |
| Table names, column names, module layout | **Go** (`AGENTS.md`) |
| Auth, RLS, tenancy, error handling, testing | **Go** (`AGENTS.md`) |
| File size, module boundaries, money types | **Go** (`AGENTS.md`) — non-negotiable |

**If Laravel does something that looks wrong, port it anyway** and record it in
`docs/modules/<module>.md`. `AGENTS.md` rule 7. We are proving parity, not
improving semantics. Improvements happen after cutover, deliberately.

---

## 0.2 Phase index

Execute **in order**. Each phase has hard dependencies on the ones before it.
Do not start a phase until the previous phase's Completion Gate passes.

| # | File | Title | Why here |
|---|---|---|---|
| 0 | `01_PHASE_0_FOUNDATION.md` | Unblock & secure | The marketplace returns zero rows and any staff account can drop tables. Nothing else can be tested until this is fixed. |
| 1 | `02_PHASE_1_VISIBILITY.md` | Institutional works + product read model | Every product/offer query in later phases depends on this filter and on `product_infos`. |
| 2 | `03_PHASE_2_COMPARE_ENGINE.md` | Discount comparison engine | The advertised flagship. Currently a paywall with nothing behind it. |
| 3 | `04_PHASE_3_PROCUREMENT.md` | Purchase request, priority engine, automation | The largest customer capability. Depends on Phase 1's read model. |
| 4 | `05_PHASE_4_INGEST.md` | Chunked import pipeline | Phases 2 and 3 need real-file-size uploads. |
| 5 | `06_PHASE_5_ADMIN.md` | Admin panel (24 missing areas) | The biggest surface gap: 183 Laravel routes vs 74. |
| 6 | `07_PHASE_6_VENDOR.md` | Vendor dashboard completion | 32 sidebar entries in Laravel, 17 in Go. |
| 7 | `08_PHASE_7_CUSTOMER.md` | Customer dashboard completion | 16 sidebar entries in Laravel, 11 in Go. |
| 8 | `09_PHASE_8_REVENUE.md` | Offers packages, sponsorships, promotions, ads | 9 tables built with zero UI. |
| 9 | `10_PHASE_9_PLATFORM.md` | 2FA, sessions, PDF, notifications, AI providers | Cross-cutting platform capability. |
| 10 | `11_PHASE_10_VERIFY.md` | Completeness verification | Systematic Laravel-vs-Go diff. **Mandatory.** |

---

## 0.3 Non-negotiable engineering rules

These come from `AGENTS.md` and are enforced by `make check`. Violating any of
them fails CI, so read them once and internalise them.

### R1 — Money never touches `float64`
Use `internal/shared/money.Amount`. Database columns are `NUMERIC(p,2)`.
`money.Amount` is `struct{ minor int64 }`, single-currency, and `ApplyPercent`
rounds half-away-from-zero. If you type `float`, stop and reconsider.

### R2 — No AI provider names outside `internal/platform/gateway/`
Modules request a *capability* (`product.match`), never a provider.
`make check-provider-isolation` fails the build otherwise. This matters in
Phases 2, 3 and 9.

### R3 — Every AI capability has a deterministic fallback
A pharmacy must be able to order and a supplier must be able to import when the
Gateway is down. Add the fallback in the same change as the capability, and add
a test that runs with the Gateway disabled.

### R4 — Tenant-scoped queries run inside `db.InTx` / `db.InReadTx`
Those set the Postgres GUC that RLS reads. Never touch `db.Pool()` from a
module. Cross-tenant access requires `database.AsSystem(ctx)`, which is
deliberately greppable — every new `AsSystem` call needs a one-line comment
saying why it is safe.

### R5 — Module boundaries
`modules/A` may not import `modules/B`. `platform/*` may not import `modules/*`.
`shared/*` imports nothing from this repo. `depguard` enforces it. When two
modules must talk, compose them in `cmd/server/routes.go` (see how `docsGate` is
built there) or define an interface in the consumer.

### R6 — 400 lines per Go file, maximum
`make check-file-size`. Split by concern: `domain.go`, `service.go`,
`repository.go`, `http/`, `jobs/`. This plan's larger features will need 4–8
files each; that is expected, not a sign you are doing it wrong.

### R7 — Never edit an applied migration
The runner checksums them. Add a new numbered migration. Wrap in
`BEGIN; … COMMIT;`. Write the `.down.sql` at the same time and make it actually
reverse the change. Verify with:
```bash
DATABASE_URL="..." go run ./cmd/migratecheck -from <N> -roundtrip
```

### R8 — Tenant-owned tables get RLS
`organization_id` column, `ENABLE ROW LEVEL SECURITY`, `FORCE ROW LEVEL
SECURITY`, and a policy using `platform.tenant_visible(organization_id)`.
Then a test proving a cross-tenant read returns zero rows. This is a CI gate.

### R9 — Carry the Arabic column comments
`COMMENT ON COLUMN … IS '<arabic>'`. For several legacy columns this is the only
documentation that exists.

### R10 — No fix ships without the check that would have caught it
And that check must be **demonstrated failing against the pre-fix tree**. Write
the test first, run it, paste the failure, then fix. A test written after the
fix tends to encode the bug.

---

## 0.4 The error-handling rule (this is where most bugs came from)

**Forbidden pattern** — it appears 60 times in `internal/ui/*.go` today and is
the single largest source of "I click and nothing happens":

```go
// NEVER DO THIS
if items, err := h.svc.List(ctx, id); err == nil {
    data.Items = items
}
```

A failed query renders an empty page with no log line and no user feedback.

**Required pattern** for every data load in a page handler:

```go
items, err := h.svc.List(ctx, id)
if err != nil {
    h.log.ErrorContext(ctx, "list items", "error", err, "org", actor.OrganizationID)
    h.renderError(w, r, err)
    return
}
data.Items = items
```

**When a partial failure is genuinely acceptable** (a secondary widget on a
dashboard), you must still log it and mark the section degraded:

```go
stats, err := h.svc.Stats(ctx, id)
if err != nil {
    h.log.WarnContext(ctx, "dashboard stats unavailable", "error", err)
    data.StatsUnavailable = true   // the template renders an error state, not silence
}
```

`data.<X>Unavailable` must render `@components.ErrorState(...)`, never an empty
list. Empty list means "there is no data". Error means "we could not load it".
The user must be able to tell those apart.

**Phase 0 Task 0.4 removes all 60 existing occurrences.** Do not add new ones.

---

## 0.5 How to add a feature in this codebase (the canonical recipe)

Every feature in every phase follows these eight steps. When a phase task says
"implement X", it means run this recipe for X.

### Step 1 — Inspect Laravel first
```bash
# Find the Livewire component
ls F:/Dawa\ 24/Laravel/app/Livewire/{Admin,Employee,Customer}/ | grep -i <feature>
# Read the component (mount, properties, render, actions)
cat F:/Dawa\ 24/Laravel/app/Livewire/<Panel>/<Component>.php
# Read its Blade view (the UI you must reproduce)
cat F:/Dawa\ 24/Laravel/resources/views/livewire/<panel>/<view>.blade.php
# Read the model (fillable, casts, relationships, scopes)
cat F:/Dawa\ 24/Laravel/app/Models/<Model>.php
# Read the table definition
sed -n "/CREATE TABLE \`<table>\`/,/ENGINE=/p" F:/Dawa\ 24/u924222867_Testv5.sql
```
Write down, before coding: the fields, the filters, the sort order, the
pagination size, the actions (buttons), the validation rules, the Arabic labels,
the empty state text, and the permission key.

### Step 2 — Migration
`db/migrations/NNN_<name>.up.sql` + `.down.sql`. Rules R7, R8, R9.
Then: `go run ./cmd/migratecheck -from <N> -roundtrip`.

### Step 3 — Domain
`internal/modules/<module>/domain.go` — types and pure rules, zero I/O.
Table-driven unit tests colocated.

### Step 4 — Repository
`internal/modules/<module>/repository.go` (interface) +
`internal/modules/<module>/postgres/<feature>.go` (SQL).
All queries inside `db.InTx` / `db.InReadTx`. Integration test against real
Postgres, including the cross-tenant zero-rows test.

### Step 5 — Service
`internal/modules/<module>/service.go` — use cases, validation, orchestration.
Returns `apperr.*` errors, never raw SQL errors.

### Step 6 — HTTP / UI
- JSON API: `internal/modules/<module>/http/handlers.go` (+ `admin.go` with
  `RequirePermission`).
- HTML page: handler in `internal/ui/<area>_handlers.go`, template in
  `internal/ui/pages/<name>.templ`.
- Register the route in the **correct audience group** in
  `internal/ui/handlers.go` (see §0.6).

### Step 7 — Wire the UI
Every button must have a target that resolves. Run the dead-target scan (§0.8)
after every phase. Zero dead targets is a completion gate.

### Step 8 — Test
Per §0.9. Then `make check`.

---

## 0.6 Audience routing — where a route goes

`internal/ui/handlers.go` has five registration functions. Putting a route in
the wrong one is a security bug.

| Function | Gate | Put a route here when |
|---|---|---|
| `RegisterPublicRoutes` | none (OptionalAuth) | signed-out visitors must reach it |
| `RegisterCustomerRoutes` | `RequireCustomer` + `RequireApproved` | only pharmacies |
| `RegisterVendorRoutes` | `RequireVendor` + `RequireApproved` | only suppliers |
| `RegisterAdminRoutes` | `RequireStaff` **+ per-route `RequirePermission`** (added in Phase 0) | platform staff |
| `RegisterSharedRoutes` | auth only | both account types, page picks its own shell via `layouts.ShellFor` |

**Rules:**
1. A page in `RegisterSharedRoutes` **must** render through
   `@layouts.ShellFor(title, nav, lang, dir, actor)`. Never name a concrete
   shell (`VendorShell`, `CustomerShell`) in a shared page — that was the bug in
   AUDIT_V3 PART 1.
2. A page in a typed group **must** name its own shell.
3. Wrong audience returns **404, not 403**. A vendor must not learn that
   `/customer/*` exists.
4. Every new admin route gets `RequirePermission` (Phase 0 Task 0.2 builds the
   mechanism; every later phase uses it).

---

## 0.7 Frontend: preserving the Laravel experience

### 0.7.1 What "preserve" means concretely

You are **not** copying Blade markup. You are reproducing, in templ:

1. **The same screens** — one templ page per Laravel Livewire screen.
2. **The same sidebar** — same entries, same order, same grouping, same Arabic
   labels. The authoritative lists are in §0.7.3.
3. **The same information on each screen** — same columns, same badges, same
   summary cards, same filters, same sort options.
4. **The same actions** — same buttons with the same Arabic labels doing the
   same thing.
5. **The same Arabic copy** — read the Blade file and reuse its strings verbatim.

You **are** free to improve: markup quality, accessibility, responsive
behaviour, component reuse, loading states, and performance. You are **not**
free to remove a field, rename a screen, reorder the sidebar, or "simplify" a
workflow.

### 0.7.2 Component library — use it, do not reinvent

`internal/ui/components/` already contains: `avatar`, `badges`, `breadcrumbs`,
`buttons`, `combobox`, `commandpalette`, `datatable`, `datepicker`,
`daterangepicker`, `drawer`, `dropdown`, `emptystate`, `error_state`,
`filedropzone`, `forms`, `icons`, `imagegallery`, `language_toggle`,
`map_picker`, `modal`, `moneydisplay`, `pagination`, `price_tag`,
`quantitystepper`, `rating`, `rating_stars`, `review_modal`, `skeleton`,
`statcard`, `stepper`, `tabs`, `theme_toggle`, `timeline`, `toast`.

**Before writing any markup, check whether a component exists.** If a listing
screen needs a table, use `components.DataTable`. If it needs an empty state,
use `components.EmptyState`. Adding a 41st bespoke table is a review failure.

If you need a component that does not exist, add it to
`internal/ui/components/` — never inline it in a page.

### 0.7.3 Authoritative sidebar definitions

These are extracted from the Laravel layouts. **Reproduce them exactly**,
including order. Items marked ✅ exist in Go today; items marked ❌ must be built.

**Vendor sidebar** — `Laravel/resources/views/components/layouts/employee.blade.php` (32 entries):

| # | Laravel path | Arabic label (read from Blade) | Go path | Status |
|---|---|---|---|---|
| 1 | `/employee/dashboard` | لوحة التحكم | `/vendor/dashboard` | ✅ |
| 2 | `/employee/organization` | بيانات المنشأة | `/settings/organization` | ✅ |
| 3 | `/employee/branches` | الفروع | `/vendor/branches` | ✅ |
| 4 | `/employee/users` | الموظفون | `/settings/employees` | ✅ |
| 5 | `/employee/roles` | الأدوار والصلاحيات | `/vendor/roles` | ✅ |
| 6 | `/employee/upload-file/first-time` | رفع الموظفين دفعة واحدة | `/vendor/team/import` | ❌ Phase 6 |
| 7 | `/employee/weekly-coverages` | التغطية الأسبوعية | `/vendor/coverage` | ⚠️ Phase 0 (broken) |
| 8 | `/employee/pharmacy-coverages` | تغطية الصيدليات | `/vendor/pharmacy-coverage` | ❌ Phase 6 |
| 9 | `/employee/choose-products` | اختيار المنتجات المتاحة | `/vendor/catalog/select` | ❌ Phase 6 |
| 10 | `/employee/products` | منتجاتي | `/vendor/products` | ✅ |
| 11 | `/employee/products/import` | استيراد المنتجات | `/vendor/ingest` | ⚠️ Phase 4 |
| 12 | `/employee/saveing-products` | منتجات التوفير | `/vendor/saving-products` | ❌ Phase 6 |
| 13 | `/employee/stocks` | المخزون | `/vendor/inventory` | ✅ |
| 14 | `/employee/warehouses` | المستودعات | `/vendor/warehouses` | ❌ Phase 6 |
| 15 | `/employee/offers` | العروض | `/vendor/offers` | ✅ |
| 16 | `/employee/offers-packages` | باقات العروض | `/vendor/offers-packages` | ❌ Phase 8 |
| 17 | `/employee/offers-packages/sponsorships` | الرعايات | `/vendor/offers-packages/sponsorships` | ❌ Phase 8 |
| 18 | `/employee/offers-packages/promotions` | الحملات الترويجية | `/vendor/offers-packages/promotions` | ❌ Phase 8 |
| 19 | `/employee/ads` | الإعلانات | `/vendor/ads` | ❌ Phase 8 |
| 20 | `/employee/orders` | الطلبات | `/vendor/orders` | ✅ |
| 21 | `/employee/orders/offers` | طلبات العروض | `/vendor/orders/offers` | ❌ Phase 6 |
| 22 | `/employee/invoices` | الفواتير | `/invoices` | ✅ |
| 23 | `/employee/payments` | المدفوعات | `/vendor/payments` | ❌ Phase 6 |
| 24 | `/employee/compare-discounts` | مقارنة الخصومات | `/compare/tool` | ⚠️ Phase 2 |
| 25 | `/employee/market-discounts` | خصومات السوق | `/vendor/market-discounts` | ❌ Phase 2 |
| 26 | `/employee/jobs` | الوظائف | `/vendor/jobs` | ✅ |
| 27 | `/employee/documents` | المستندات | `/vendor/documents` | ✅ |
| 28 | `/employee/policies` | السياسات | `/vendor/policies` | ❌ Phase 6 |
| 29 | `/employee/social-media` | وسائل التواصل | `/vendor/social-media` | ❌ Phase 6 |
| 30 | `/employee/highlight-sections` | الأقسام المميزة | `/vendor/storefront` | ✅ |
| 31 | `/employee/activities` | سجل النشاطات | `/vendor/activities` | ❌ Phase 6 |
| 32 | `/employee/user-organization` | عضوية المنشآت | `/settings/organization` | ✅ |

**Customer sidebar** — `Laravel/resources/views/components/layouts/customer.blade.php` (16 entries):

| # | Laravel path | Go path | Status |
|---|---|---|---|
| 1 | `customer.dashboard` | `/customer/dashboard` | ✅ |
| 2 | `/customer/c-panel` | `/customer/cpanel` | ❌ Phase 7 |
| 3 | `/customer/offers` | `/offers` | ✅ |
| 4 | `/customer/suppliers` | `/suppliers` | ✅ |
| 5 | `customer.followed-suppliers` | `/suppliers/followed` | ✅ |
| 6 | `/customer/compare-discounts` | `/compare/tool` | ⚠️ Phase 2 |
| 7 | `/customer/saveing-products` | `/customer/saving-products` | ❌ Phase 7 |
| 8 | `/customer/purchase-request` | `/customer/purchase-request` | ❌ Phase 3 |
| 9 | `/customer/purchase-request/products` | `/customer/purchase-request/products` | ❌ Phase 3 |
| 10 | `customer.orders` | `/orders` | ✅ |
| 11 | `customer.orders.offers` | `/orders/offers` | ❌ Phase 7 |
| 12 | `/customer/job-opportunities` | `/jobs` | ✅ |
| 13 | `/customer/documents` | `/customer/documents` | ✅ |
| 14 | `/customer/user-organization` | `/settings/organization` | ✅ |
| 15 | `customer.wallet` | `/wallet` | ✅ |
| 16 | `customer.profile` | `/settings/profile` | ✅ |

**Admin sidebar** — `Laravel/resources/views/components/layouts/admin.blade.php`
(~50 entries). Full mapping is in `06_PHASE_5_ADMIN.md` §5.2.

### 0.7.4 Mandatory UI states

Every list screen and every form must handle **all five** states. A screen
missing any of these is incomplete:

| State | Requirement |
|---|---|
| **Loading** | `@components.Skeleton(...)` for HTMX-swapped regions; server-rendered pages need no spinner but must not flash empty |
| **Empty** | `@components.EmptyState(title, description, actionLabel, actionHref)` — Arabic copy taken from the Laravel screen. Never render a bare empty `<table>`. |
| **Error** | `@components.ErrorState(...)` — see §0.4. Distinct from empty. |
| **Success** | `@components.Toast(...)` via `h.redirectWithNotice(w, r, path, "success", "<arabic>")` |
| **Partial/degraded** | section-level error state, page still renders |

### 0.7.5 Forms — mandatory checklist

Every form you create must have all of these:

- [ ] `method="POST"` and an `action` that resolves to a registered route
- [ ] Server-side validation returning `apperr.Validation` with field-level messages
- [ ] Field errors rendered inline next to the field, in Arabic
- [ ] Required fields marked visually and with `required`
- [ ] Submit button disabled during submission (Alpine `x-data="{busy:false}"`)
- [ ] Success → redirect with a toast, never a blank 200
- [ ] Failure → re-render with the submitted values preserved
- [ ] CSRF: matches whatever the existing forms in `internal/ui/pages/` do — check `settings.templ` and copy it
- [ ] RTL layout correct (labels right-aligned, icons mirrored)

### 0.7.6 Lists — mandatory checklist

- [ ] Pagination via `@components.Pagination` — **default page size matches Laravel's** (read `$paginate` / `->paginate(N)` in the Livewire component)
- [ ] Search box if Laravel has one, searching the same fields
- [ ] Filters matching Laravel's filter set exactly (status, date range, org, city…)
- [ ] Sort options matching Laravel's
- [ ] Row actions matching Laravel's buttons
- [ ] Bulk actions if Laravel has them
- [ ] Total count displayed if Laravel displays it
- [ ] Export button if Laravel has one

### 0.7.7 Localization

- Every user-facing string is bilingual. Follow the existing pattern in
  `internal/ui/pages/*.templ`: `if lang == "ar" { ... } else { ... }` or the
  `i18n` helper — check `internal/shared/i18n` and match whatever the
  neighbouring pages do.
- **Arabic is primary.** `dir="rtl"` is the default. Design RTL first, then
  verify LTR.
- Bilingual DB content uses `{"ar":"...","en":"..."}` JSONB.
- Numbers, currency and dates use the existing `components.MoneyDisplay` and the
  locale helpers. Do not hand-format.
- New strings: take the Arabic from the Laravel Blade file verbatim. Do not
  invent or translate. If the English is missing in Laravel, write a reasonable
  English string.

### 0.7.8 Responsive

- Sidebar collapses below 1024px (match the existing `layouts/*.templ` behaviour).
- Tables become card lists below 768px, or scroll inside `overflow-x:auto`.
- Touch targets ≥ 44px.
- Test at 375px, 768px, 1280px before marking a screen complete.

---

## 0.8 Verification commands (run these constantly)

Save these; every phase's Completion Gate references them.

```bash
# 1. Everything CI runs
make check

# 2. Migration round-trip (from the first migration you added in this phase)
DATABASE_URL="postgres://..." go run ./cmd/migratecheck -from <N> -roundtrip

# 3. Dead template targets — MUST be 0 (excluding /ready)
#    Compares every action/hx-post/href in templates against registered routes.
grep -ohE '"/[^"]*"' internal/ui/handlers.go internal/ui/static.go internal/ui/upload_handlers.go \
  | tr -d '"' | sort -u > /tmp/routes.txt
grep -rhoE '(action|hx-post|hx-get|href)="(/[^"]*)"' internal/ui/pages/*.templ \
  internal/ui/components/*.templ internal/ui/layouts/*.templ \
  | sed -E 's/.*="([^"]*)"/\1/' | sed 's/?.*//' | sort -u > /tmp/targets.txt
# then diff with the regex matcher documented in PARITY_AUDIT_V4 PART 0

# 4. Unregistered handlers — MUST be 0
grep -ohE "^func \(h \*UIHandler\) ([A-Za-z0-9_]+)\(w http" internal/ui/*.go \
  | sed -E 's/func \(h \*UIHandler\) //;s/\(w http//' | sort -u > /tmp/handlers.txt
grep -ohE "h\.[A-Za-z0-9_]+\)" internal/ui/handlers.go | sed 's/h\.//;s/)//' | sort -u > /tmp/registered.txt
comm -23 /tmp/handlers.txt /tmp/registered.txt

# 5. Swallowed errors — MUST be 0 in internal/ui after Phase 0
grep -rn 'err == nil {' internal/ui/*.go | wc -l

# 6. Tables with no route touching them
#    For each new table, grep the module for its name; if nothing, you built dead weight.

# 7. AsSystem audit — every call needs a justifying comment
grep -rn "database.AsSystem" internal/ --include=*.go
```

---

## 0.9 Testing requirements (per feature, non-negotiable)

For **every** feature in every phase, you must produce all six:

| # | Test | Location | Proves |
|---|---|---|---|
| T1 | **Domain unit test**, table-driven | colocated `*_test.go` | the business rule matches Laravel, including edge cases |
| T2 | **Repository integration test** against real Postgres | `postgres/*_test.go` | the SQL binds, returns the right shape, and handles NULLs |
| T3 | **Cross-tenant isolation test** | same file | org A reading org B's rows returns **zero rows**, not an error |
| T4 | **Route audience test** | `test/route_audience_test.go` | the route is in exactly one audience group; wrong audience gets 404 |
| T5 | **Permission test** (admin features) | `test/admin_guard_test.go` | a staff user *without* the permission gets 404 |
| T6 | **Handler test** | `http/handlers_test.go` or `internal/ui/handlers_test.go` | the page renders, the form submits, validation errors surface |

Plus, where applicable:
- **T7 — AI fallback test**: Gateway disabled, result still usable (rule R3).
- **T8 — Money exactness test**: exact-value assertions, no approximate compare.

**Rule R10 applies:** write the test, run it, confirm it fails, then implement.

---

## 0.10 Definition of Done — per feature

A feature is complete only when **every** box is ticked. Do not mark a task done
otherwise, and do not batch this check to the end of a phase.

**Data layer**
- [ ] Migration written, `.down.sql` reverses it, `migratecheck -roundtrip` passes
- [ ] RLS enabled on tenant-owned tables, policy uses `platform.tenant_visible`
- [ ] Arabic column comments carried across
- [ ] Foreign keys and indexes match Laravel's access patterns
- [ ] `test/schema_consistency_test.go` and `test/check_constraints_test.go` pass

**Domain / service**
- [ ] Business rule matches Laravel *exactly* (deviations recorded in `docs/modules/<m>.md`)
- [ ] Money uses `money.Amount`
- [ ] Validation returns `apperr.Validation` with field-level Arabic messages
- [ ] File under 400 lines
- [ ] No cross-module import

**HTTP / UI**
- [ ] Route registered in the correct audience group
- [ ] `RequirePermission` on admin routes
- [ ] Page renders all five states (§0.7.4)
- [ ] Every button/form target resolves (dead-target scan = 0)
- [ ] Sidebar entry added in the Laravel-equivalent position
- [ ] Arabic + English strings present
- [ ] RTL verified; responsive verified at 375/768/1280
- [ ] No `err == nil {` swallow

**Tests**
- [ ] T1–T6 written and passing
- [ ] Each was demonstrated failing before the fix (R10)

**Docs**
- [ ] `docs/modules/<module>.md` updated: entities, invariants, legacy quirks
- [ ] Any Laravel behaviour deliberately not ported is recorded with a reason

---

## 0.11 When something is unclear — the escalation rule

**Do not guess. Do not invent product behaviour.**

If the audit or this plan is ambiguous about how something should work:

1. **Read the Laravel implementation.** The Livewire component + its Blade view +
   the model. Nine times out of ten the answer is there.
2. **Read the live site**: https://test-v4.dawa24.com — and its `/what-in`
   reference page, which is the platform's own product spec.
3. **Read the SQL dump** for the exact column semantics:
   `F:\Dawa 24\u924222867_Testv5.sql`.
4. If it is *still* ambiguous, **write the question into
   `docs/PLAN_V5/OPEN_QUESTIONS.md`** with:
   - the file and line in Laravel you read
   - the two or more interpretations you are choosing between
   - which one you picked and why
   - what would need to change if the other is correct

   Then **implement your best interpretation and continue**. Do not stall the
   phase. Do not silently omit the feature.

**Never** mark a feature complete-but-stubbed. If you cannot finish it, leave it
unstarted and record why in `OPEN_QUESTIONS.md`. A stub that renders is worse
than a missing page, because it looks done.

---

## 0.12 Progress tracking

Maintain `docs/PLAN_V5/PROGRESS.md` with one row per task across all phases:

```markdown
| Phase | Task | Status | Commit | Tests | Notes |
|-------|------|--------|--------|-------|-------|
| 0 | 0.1 Vendor coverage write path | done | abc1234 | T1-T6 ✅ | |
| 0 | 0.2 Admin permission gates | in-progress | | T5 written, failing | |
```

Statuses: `not-started` / `in-progress` / `done` / `blocked` / `deferred`.
`deferred` requires an entry in `OPEN_QUESTIONS.md`.

Update it **after every task**, not at the end of a phase.

---

## 0.13 Commit discipline

- One commit per task, not per phase.
- Message format:
  ```
  <module>: <what changed>

  <why, in 2-4 lines, referencing the Laravel behaviour being matched>

  Phase <N> Task <N.M>. Tests: T1-T6.
  ```
- Never commit with `make check` failing.
- Never commit generated `*_templ.go` without the `.templ` source.

---

## 0.14 What must NOT be touched

These are correct today. Changing them is a regression, and Phase 10 will check:

| Component | File | Why |
|---|---|---|
| Audience gate semantics (404 not 403) | `internal/platform/authctx/audience.go` | Proven correct by test; a vendor must not learn `/customer/*` exists |
| `layouts.ShellFor` dispatcher | `internal/ui/layouts/shell_for.templ` | Fixes the shared-page shell bug; all 14 shared pages depend on it |
| Coverage visibility query semantics | `internal/modules/promo/postgres/visibility.go` | The single canonical expression of the coverage rule. You may *add* the institutional filter in Phase 1; you may not fork the query. |
| `docsGate` composition | `cmd/server/routes.go:96` | Fails closed on missing mandatory documents |
| Order → offer model | `db/migrations/063_order_model.up.sql` | Faithful to Laravel's 13-status enum |
| `money.Amount` | `internal/shared/money` | Single-currency by design; do not add a currency field |
| `cmd/migratecheck` | — | Your safety net for every migration |
| Review criteria schema | `db/migrations/075_*` | Already ahead of Laravel |

---

## 0.15 Start here

1. Read `docs/PARITY_AUDIT_V4.md` end to end.
2. Read `AGENTS.md`.
3. Create `docs/PLAN_V5/PROGRESS.md` and `docs/PLAN_V5/OPEN_QUESTIONS.md`.
4. Open `01_PHASE_0_FOUNDATION.md` and begin at Task 0.1.
