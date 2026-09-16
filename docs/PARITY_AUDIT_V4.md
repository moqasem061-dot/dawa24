# Dawa24 — Laravel → Go Parity Audit (V4)

**Date:** 2026-08-19
**Reference system (source of truth):** `F:\Dawa 24\Laravel` + live https://test-v4.dawa24.com/what-in + `u924222867_Testv5.sql`
**System under audit:** `F:\Dawa 24\dawa24-store` @ `7ab11a6`

This audit answers one question: *what would still be missing if we switched off the
Laravel platform tomorrow?* It is organised as the ten categories requested, and every
claim below is traceable to a file and a line — nothing here is inferred from naming.

---

## PART 0 — Method

| What | How it was measured |
|---|---|
| Laravel route surface | `routes/{web,admin,vendor,customer,api}.php`, read in full |
| Laravel screen surface | `app/Livewire/**` component files (353) |
| Laravel business logic | `app/Services/**` (14,009 lines), `app/Jobs`, `app/Observers`, `app/Models` (114) |
| Laravel schema | `CREATE TABLE` extraction from `u924222867_Testv5.sql` → **148 tables** |
| Laravel intent | the `/what-in` reference page (its own product spec, seeded from `WhatInContent::defaultData()`) |
| Go route surface | `internal/ui/handlers.go` register functions + every module's `RegisterRoutes` |
| Go schema | `CREATE TABLE` minus `DROP TABLE` across `db/migrations/*.up.sql` → **112 tables** |
| Go dead wiring | every `action=`/`hx-post`/`href` target in 75 `.templ` files matched against 209 registered paths |
| Go dead handlers | 220 `UIHandler` methods matched against the registration list |

Two mechanical results are worth stating up front because they shape everything else:

* **Dead template targets: 1** (`/ready`, a health path registered outside the UI mux).
* **Unregistered handlers: 1** (`JobApplySubmit`).

**The Go application is internally coherent.** What is built, is wired. The gap is not
rot — it is *absence*. The Go system implements roughly a third of the Laravel surface,
and several of the pieces it does implement cannot function because their upstream
feeder screen was never built.

---

## PART 1 — The scoreboard

| Surface | Laravel | Go | Coverage |
|---|---:|---:|---:|
| Admin routes | 183 | 26 GET + 48 POST | **~14% of pages** |
| Vendor routes | 82 | 21 GET + 20 POST | **~26%** |
| Customer routes | 51 | 10 GET + 19 POST | **~20%** |
| Shared/account routes | — | 21 GET + 29 POST | — |
| Public routes | 38 | 30 GET + 7 POST | ~79% |
| Screens (Livewire vs templ) | 353 | 75 | **21%** |
| DB tables | 148 | 112 | 76% (but see PART 6) |
| Domain services | 29 files / 14,009 lines | 16 modules | — |
| Background jobs | 8 | 0 UI-reachable | — |
| Admin permission keys enforced on pages | every admin route | **0** | **0%** |

---

## PART 2 — What already exists and is correctly implemented

These are genuinely done, and several are better than Laravel. Do not touch them.

| Area | Evidence |
|---|---|
| **Account-type isolation** | `internal/platform/authctx/audience.go` — four gated groups, wrong audience → 404 not 403. Laravel shares `/admin/*` across `role:admin,manager` and relies on per-route middleware it sometimes omits. |
| **Tenant isolation via RLS** | `platform.tenant_visible()` + `database.WithTenant` / `AsSystem`. Laravel has *no* row-level isolation — every Livewire component hand-writes `where('organization_id', …)` and any omission is a cross-tenant leak. |
| **Schema organisation** | 15 Postgres schemas vs one flat MySQL namespace. Laravel's 148 tables include a dozen near-duplicates (`orders`+`main_orders`+`adv_orders`, `highlight_sections`+`organization_highlight_sections`, the `saveing-`/`saving-` route twins). |
| **Order → offer model** | `db/migrations/063_order_model.up.sql` reproduces Laravel's 13-value status enum exactly, plus `offer_id` / `branch_id` / `vendor_branch_id` and `offer_product_id` line snapshots. Faithful. |
| **Coverage visibility query** | `internal/modules/promo/postgres/visibility.go` — one canonical query, Haversine via `platform.distance_meters()`. Laravel scatters this rule across `Dashboard.php`, `PurchaseRequestProducts.php` and `ShowPurchaseRequestSupplier.php` with *different* semantics in each. |
| **Documents gate** | `cmd/server/routes.go:96` `docsGate` — fails closed, blocks checkout and offer publishing on missing mandatory documents. |
| **Module API permission gates** | 13 modules each `RequirePermission("<module>.admin")` on their admin sub-router, enforced by `test/admin_guard_test.go`. |
| **Migration dry-run tooling** | `cmd/migratecheck` — applies cumulatively in one transaction, always rolls back. Laravel has nothing equivalent. |
| **Reviews schema** | `org.review_criteria` + `org.review_ratings` seeded (migration 075). Laravel's `organization_reviews` is a single scalar. Go is *ahead* here. |

**Resolved since AUDIT_V2/V3** (verified in this pass — do not re-report):
`visibility.go` now binds `$1..$5` with 5 args (A4 closed) · `layouts.ShellFor` exists and
all 14 shared pages use it (V3 PART 1 closed) · `ListEmployees` now uses `AsSystem`
(V3 PART 6 root cause closed) · `review_criteria` seeded (V3 PART 5.1 schema half closed).

---

## PART 3 — Exists, but behaves differently from Laravel

### 3.1 Permissions are enforced on the API and *not* on the admin UI — P0 security

Laravel gates **every** admin page individually:

```php
Route::get('/developer/sql-console', AdminSqlConsole::class)
    ->middleware('permission:sql-console-developer');
Route::get('/products', AdminProducts::class)->middleware('permission:products_view');
Route::get('/wallets', AdminWallets::class)->middleware('permission:payments_view');
```

Go gates the whole admin group with **one** check (`cmd/server/routes.go:196`):

```go
uiRouter.Use(authctx.RequireStaff(log))
```

and `RequireStaff` tests exactly one boolean (`audience.go:47`):

```go
if !actor.IsStaff { notFound(w, r); return }
```

`RequirePermission` exists (`authctx/middleware.go:12`) and is used **15 times — all on
JSON APIs, zero times on the 74 admin UI routes.**

Concrete consequence: the `support` role (seeded in `002_identity`) passes `IsStaff`, so a
support agent can open `/admin/developers`, POST to `/admin/developers/sql`, and reach
`platform_admin/postgres/repository.go:469`, which runs the statement inside
`database.AsSystem` — RLS bypassed. Non-`SELECT` statements go to `tx.Exec`.
**Any staff account can drop any table.** Laravel's equivalent requires
`sql-console-developer`, held only by the developer role.

### 3.2 Institutional-work product filtering is not applied anywhere

Laravel treats this as a first-class visibility rule with two documented modes
(`Laravel/docs/institutional_work_filter.md`):

* *Simple* — product visible if `institutional_work_ids` intersects the user's works, **plus** unrestricted products.
* *WithConnections* — resolve `institutional_work_connections.from → to` first, then intersect; **no fallback for unrestricted products**.

Laravel applies mode 1 on `/customer/dashboard` and mode 2 on
`/customer/purchase-request/products` and `/customer/purchase-request/supplier/{id}`.

Go has the tables (`org.institutional_works`, `org.institutional_work_connections`,
`org.branch_institutional_works`) and an admin CRUD screen — and applies the filter to
**zero** product or offer queries. `grep -rn institutional internal/modules/catalog
internal/modules/promo` returns nothing. Every customer sees every product regardless of
institutional group. This is a data-visibility regression, not merely a missing feature.

### 3.3 Two order systems collapsed into one — rating/review data has no home

Laravel runs `orders`/`order_items` (direct; carries `rating`, `review`, `rated_at`,
`delivered_at`) *alongside* `main_orders`/`adv_orders` (offer-based; `adv_orders` carries
its own per-line `rating`). Go kept only the offer-based shape. The order-level
`rating` / `review` / `rated_at` columns were not carried onto `commerce.orders`, so
post-delivery order rating — the thing that feeds `organization_reviews` — has no
storage path.

### 3.4 Permission taxonomy diverged

Laravel: ~50 feature-scoped admin keys (`products_view`, `offer_sponsorships_update`,
`temp_warehouses_view`) plus a separate `supplier_permissions` set for org roles.
Go: ~35 module-scoped keys (`catalog.product.view`, `promo.admin`). The Go set is cleaner
but **strictly coarser** — there is no equivalent of `offer_plans_view` vs
`offer_sponsorships_view` vs `offer_promotions_view` vs `offer_clicks_analytics_view`.
Any migration of Laravel role assignments will over-grant.

### 3.5 `ensureDeveloperTables` creates schema at request time

`internal/modules/platform_admin/postgres/repository.go:470` calls `ensureDeveloperTables`
inside the SQL-console transaction. Schema created outside `db/migrations` is invisible to
`migratecheck` and has no down path.

---

## PART 4 — Exists, but incomplete or broken

### 4.1 Vendor weekly coverage is read-only *and* always empty — P0, breaks the marketplace

`internal/ui/vendor_handlers.go:1305`:

```go
var coverages []*workflow.WeeklyCoverage
var bands []*org.DeliveryBand

if h.orgSvc != nil && actor.OrganizationID > 0 {
    if b, err := h.orgSvc.GetDeliveryBands(ctx, actor.OrganizationID); err == nil {
        bands = b
    }
}
// coverages is never assigned
pages.VendorCoverage(coverages, bands, lang, dir).Render(ctx, w)
```

`coverages` is declared, never populated, and rendered. And
`internal/ui/pages/vendor_coverage.templ` (106 lines) contains **zero** `form`, `hx-`,
`@click` or `button` — it is a static panel.

Only one route is registered: `r.Get("/vendor/coverage", …)`. The write endpoint
`POST /api/v1/workflow/branches/{id}/coverage` exists and works — **nothing calls it.**

Chain of consequence:

1. No vendor can create a `workflow.weekly_coverages` row through the product.
2. `ListOffersVisibleTo` INNER JOINs `workflow.weekly_coverages`.
3. → **every customer offer listing returns zero rows, permanently.**

The customer-facing marketplace is structurally dead until a vendor can save coverage.
Laravel has 7 coverage screens (vendor add/edit/show/branch, pharmacy-coverage ×2, admin ×4).

### 4.2 The discount-comparison engine is a paywall with nothing behind it — P0

The `/what-in` page names this the platform's flagship: *"محرك مقارنة الخصومات والأسعار
الذكي"*, granted to **all three** roles, with a documented four-step process (upload →
manual column mapping → supplier-vs-supplier comparison → AI drug-name matching at 99%
accuracy, under 8-file/22-file archive limits).

Laravel implements it in 11 Livewire components (`CompareDiscounts`,
`CompareDiscountsMarketing`, `CompareDiscountsPlan`, `CompareDiscountsPlanRequest`,
`CompareDiscountsShowPlan`, `MarketDiscounts`, `UploadGlobalDiscounts`, plus customer
twins), 6 tables and 3 services (`ProductMatcher` 939 lines, `ColumnDetector` 814,
`ArabicNormalizer` 127).

Go has `internal/ui/compare_handlers.go` — **75 lines, three functions**:

```go
func (h *UIHandler) CompareToolPage(w http.ResponseWriter, r *http.Request) {
    entitled := false
    if actor, ok := authctx.From(ctx); ok && h.billSvc != nil {
        if has, _, err := h.billSvc.CheckEntitlement(ctx, actor.UserID, "compare"); err == nil {
            entitled = has
        }
    }
    pages.CompareToolPage(lang, dir, entitled).Render(ctx, w)   // takes no data
}
```

It renders a pricing page, takes a subscription, then renders a tool that receives no data
and has no upload. `grep -r compare_discount` across `internal/` and `db/migrations/`
returns **nothing** — all 6 tables are absent. Today you can charge a customer for a
feature that does not exist.

### 4.3 Two-factor authentication cannot be enabled

`identity.user_mfa` exists. `AdminResetMFA` exists and is routed. There is **no enrollment
screen and no login challenge** — `grep -rn "2fa\|totp\|TwoFactor" internal/ui` returns
nothing. Laravel has `Google2FAService` (124 lines), a `/2fa-challenge` route and
`App\Livewire\Auth\TwoFactorChallenge`. Admin can reset a second factor no user can set.

### 4.4 Vendor ingest wizard is still mostly decorative

Ten ingest API endpoints are complete (presign, session, mapping, commit, cancel, SSE).
`internal/ui/pages/vendor_ingest.templ` now has 2 form/fetch hooks (up from 0), but step
transitions are still Alpine `@click="step = 3"` with no server round-trip. The
column-mapping step — the part `ColumnDetector.php` exists to power — has no counterpart.

### 4.5 60 swallowed-error sites in the UI layer

`grep -c 'err == nil {' internal/ui/*.go` → **60** (was 34 at AUDIT_V3; it grew). Each is
`if x, err := …; err == nil { use(x) }` — a failed query renders an empty page with no log
line. This single habit produces most "I click and nothing happens" reports, and it is
actively spreading.

### 4.6 Admin pages built but unreachable

`RegisterAdminRoutes` registers 26 GET pages. `internal/ui/layouts/admin.templ` links 13.
Orphaned: `/admin/audit`, `/admin/messages`, `/admin/content`, `/admin/translations`,
`/admin/jobs`, `/admin/policies`, `/admin/finder`, `/admin/services`, `/admin/plans`,
`/admin/vendors`, `/admin/suppliers`. Working screens nobody can navigate to.

### 4.7 `JobApplySubmit` is written but not routed

`internal/ui/jobs_handlers.go:72`. `/jobs` and `/jobs/{id}` render publicly; the apply
button has no endpoint. `hr.job_applications` will stay empty.

---

## PART 5 — Completely missing

### 5.1 Customer side

| Missing capability | Laravel implementation | Impact |
|---|---|---|
| **Automatic Purchase Request** (طلب شراء أوتوماتيكي) | `EnhancedAutomationService` **4,696 lines**, `PharmaceuticalAutomationService` 625, `AutomationRequest`/`AutomationPrevious`/`AutomationRequestComponent`, `ProcessAutomationFile` + `ProcessEnhancedAutomationJob`, chunked upload controller, `/customer/automation-request/template` | The single largest business capability in the platform. Zero Go equivalent. |
| **Purchase Priority Engine** | `PurchasePriorityEngineService` 379 lines; 4 priority flags (highest discount / lowest price / fastest delivery / preferred suppliers only) + `budget_constraint`; reads the `product_infos` read-model; writes `matched_products`, `ranking_results`, `recommendations` | Table `workflow.purchase_priority_engines` exists — **no service, no handler, no screen**. |
| **Purchase Request flow** | `/customer/purchase-request`, `/purchase-request/supplier`, `/supplier/{id}`, `/products`, `/previous` (5 screens) | Absent. `commerce.quote_requests` is the nearest table; only reachable via `/suppliers/{id}/quote`. |
| **Saving Products** (منتجات التوفير) | `saving_products` table; customer + vendor + admin screens; per-user and per-org variants; import landing | Table **deliberately dropped** by migration 071 on the claim that "its semantics are superseded by promo offers". `/what-in` lists it as a distinct customer pillar alongside Offers. |
| **Order optimisation** | `OrderOptimizationService` 305, `AutomatedOrderOptimizationService` 370, `GeolocationSupplierOptimizer` 278, `Customer/OrderOptimization` | Absent. |
| **Guest order tracking** | `Front/GuestOrderTracking`, `/tracking` | Absent. |
| **Supplier storefront sub-pages** | `Customer/Supplier/{About,Header,OurBranches,Policies,Products,Reviews}` | Go has one flat `/suppliers/{id}`. |
| **CPanel** | `Customer/Cpanel`, `/customer/c-panel` | Absent. |
| **Compare / market discounts** | see 4.2 | Shell only. |

### 5.2 Vendor side

| Missing capability | Laravel | Go |
|---|---|---|
| **Offer packages / sponsorships / promotions** | `/vendor/offers-packages`, `/sponsorships`, `/sponsorships/{id}`, `/promotions`; `OfferSponsorshipService` 194, `OfferRotationEngine` 110, `OfferAnalyticsService` 71, `OfferViewTrackingService` 87, `OfferClickTrackingService` 53 | **All 6 tables exist; zero routes.** |
| **Advertising** | `/vendor/ads` + add/edit/show | `promo.ads`, `ad_plans`, `ad_clicks` present, **no UI at all**, vendor or admin |
| **Weekly coverage editing** | 7 screens | see 4.1 |
| **Pharmacy coverage** | `PharmacyEmployeeWeeklyCoverage`, `…ShowWeeklyCoverage` | absent |
| **Temp warehouses (مخازن مؤقتة)** | `user_temparte_warehouses`, `plan_temparte_warehouses`, `father_user_…`, `user_plan_…`; ~12 admin routes; `WarehouseLifecycleService` 396; `ProcessWarehouseBatch`/`ProcessWarehouseFile` jobs | `inventory.temp_warehouses` exists; **no screen, no lifecycle service, no plan/request flow** |
| **Institutional work management** | `EmployeeInstitutional`, `/vendor/institutional-work` | vendor side absent (admin CRUD exists) |
| **Earnings** | `/vendor/earnings/order`, `/earnings/offers` | absent |
| **Activities log** | `/vendor/activities`, `EmployeeActivityObserver`, `employee_activities` | absent |
| **Job categories** | `/vendor/job-categories` | `hr.job_categories` exists, no screen |
| **Policies / social media / highlight sections** | `/vendor/policies`, `/social-media`, `/highlight-sections` | tables exist; only `/vendor/storefront` partially covers the last |
| **Bulk employee upload** | `FirstTimeUploadUsers`, `EmployeeFastAdd` | absent |
| **Not-approved-yet screen** | `/vendor/not-approved-yet` | Go redirects to `/onboarding/pending` — acceptable substitute |

### 5.3 Admin side — the largest gap

Laravel's admin sidebar has ~50 entries across 183 routes. Go's has 13.
**Entirely absent admin capability:**

| # | Laravel admin area | Routes | Go |
|---|---|---:|---|
| 1 | **Trash / soft-delete recovery** (`trash-list`, `deletes-lists` × model × id) | 6 | none — `/what-in` lists this as a headline admin pillar |
| 2 | **Temp-warehouse administration** (user/plan/import/my/admins/org/customer + requests) | 14 | none |
| 3 | **Saving-products administration** (org/user/import landing/upload) | 10 | none (table dropped) |
| 4 | **Offers-packages hub** (packages/sponsorships/promotions/views/clicks × index+show) | 11 | none |
| 5 | **Ads + ad plans + session plans** | 15 | none |
| 6 | **Full user panel** (`full-user`, new-clients, customer-list, vendor-list, admin-list, each ×5 actions) | 22 | one flat `/admin/users` |
| 7 | **Chat tree + chat history** | 3 | none |
| 8 | **Error logs** (`full-error-logs` index+show) | 2 | service methods exist, **no page** |
| 9 | **Activity logs** (`full-activity-logs`, `activities`, `employee-activities`) | 4 | `/admin/audit` exists but is unlinked |
| 10 | **AskFor** (admin requests documents from an org) | 2 | none |
| 11 | **Contact-us inbox** | 2 | table exists, `/admin/messages` unlinked |
| 12 | **Want-delete** (account deletion queue) | 2 | POST handlers exist, **no page** |
| 13 | **Branch products / branch users** | 4 | none |
| 14 | **Earnings reporting** | 2 | none |
| 15 | **System resources** (`system-page`, per-system) | 2 | table exists, no screen |
| 16 | **Plans info / plan types / plan features / subscriptions** | 5 | `/admin/plans` only |
| 17 | **Product children / adv products / apis products / image import** | 6 | none |
| 18 | **Import products (admin, chunked)** | 3 | `/admin/products/import` — single-shot, no chunking |
| 19 | **User addresses** | 2 | none |
| 20 | **Roles / admin-roles / admin-permissions** | 4 | none — **there is no way to edit a role in the Go admin** |
| 21 | **Countries / currencies / brands / categories / stocks / warehouses** | 8 | `/admin/cities` only |
| 22 | **Invoices / payments / wallets** | 3 | none |
| 23 | **First-look onboarding** | 1 | none |
| 24 | **AI test console** | 1 | folded into `/admin/developers` ✓ |

### 5.4 Cross-cutting

| Capability | Laravel | Go |
|---|---|---|
| **Arabic PDF invoices** | `/pdf/{orderId}`, `resources/views/pdfs/` | absent — `/what-in` grants it to all three roles |
| **Multi-session / device tracking** | `user_sessions`, `user_session_histories`, `compare_discount_user_sessions`, device UUID, `SessionService` 271 | `identity.session_plans` table only; no device tracking, no session list beyond revoke |
| **Session-plan purchase/request flow** | `session_plans`, `session_plan_requests`, admin CRUD + requests queue | `POST /settings/security/plan/{id}` exists; no admin side, no request table |
| **Background job pipeline** | 8 queued jobs, chunked imports, `supervisor-worker.conf` | worker binary exists; no chunked import path |
| **AI providers registry** | `ai_providers` table + 6 provider services (OpenAI/Claude/Gemini/DeepSeek/Groq/OpenRouter) | gateway exists; **no `ai_providers` table**, no provider fallback cascade |
| **Product/offer importance ranking** | `product_importants`, `offer_importants` | absent |
| **Subscription history** | `subscription_histories`, `subscription_users`, `user_plan_histories`, `plan_types` | absent |
| **SQL query history** | `sql_query_histories` | created ad-hoc by `ensureDeveloperTables`, not in migrations |
| **`report_issues`** | table + flow | table exists in Go, **no route** |
| **Visitor analytics reporting** | `visitors` + admin views | middleware records; `/admin/analytics` exists but is unlinked |

---

## PART 6 — Database gap

**Laravel 148 tables → Go 112.** Removing 14 framework/infra tables (`cache`,
`cache_locks`, `jobs`, `job_batches`, `failed_jobs`, `migrations`, `sessions`,
`password_reset_tokens`, `personal_access_tokens`, `telescope_*` ×3) and 5 read-model
views (`all_products`, `basic_products`, `products_view`, `organizations_overview`,
`product_search_index`) leaves **~129 business tables → 112**.

### 6.1 Tables with no Go counterpart

| Laravel table | Purpose | Consequence of absence |
|---|---|---|
| `compare_discount_plans` | comparison-engine plans | flagship feature unbuildable |
| `compare_discount_plan_features` | per-plan limits (8/22 file archive caps) | archive policy unenforceable |
| `compare_discount_plan_requests` | vendor requests a plan | — |
| `compare_discount_plan_subscriptions` | active subscriptions | billing currently reuses `billing.plans` with a `"compare"` string — no limits |
| `compare_discount_plan_subscription_users` | seat assignment | — |
| `compare_discount_user_sessions` | device/session cap per seat (device_name, platform, browser, ip, country, city) | the multi-device protection advertised on `/what-in` cannot work |
| `saving_products` | منتجات التوفير | dropped by migration 071 — **down migration cannot restore rows** |
| `product_clients` | customer's own product names | folded into `customer_product_mappings`; `user_id` deliberately dropped, so per-user mappings are lost |
| `ai_providers` | provider registry + cascade order | no failover config |
| `full_error_logs` | error diagnostics | service methods exist with nowhere to write |
| `sql_query_histories` | console audit | created outside migrations |
| `plan_types` | plan taxonomy | — |
| `subscription_histories`, `subscription_users`, `user_subscriptions`, `user_plan_histories` | subscription lifecycle | `billing.subscriptions` has no history |
| `session_plan_requests` | session-plan purchase requests | `POST /settings/security/plan/{id}` has no queue |
| `plan_temparte_warehouses`, `user_plan_temparte_warehouses`, `father_user_temparte_warehouses` | temp-warehouse plan hierarchy | — |
| `branch_weekly_locations` | per-branch weekly *location* (distinct from coverage radius) | coverage model is coarser than Laravel's |
| `employee_activities` | per-employee action log (`EmployeeActivityObserver`) | vendor activity screen impossible |
| `employee_institutional_works` | user ↔ institutional work | **`PurchasePriorityEngineService` reads this table directly** — the filter in §3.2 has no source data |
| `offer_importants`, `product_importants` | manual ranking | ordering falls back to `created_at` |
| `offer_package_features` | package entitlements | — |
| `customer_product_mappings.user_id` *(column)* | per-user mapping | lost in migration 071 |
| `orders.rating` / `.review` / `.rated_at` *(columns)* | order rating | see §3.3 |

### 6.2 Tables present in Go with **no route touching them**

Built and empty forever:

`promo.offer_packages` · `promo.offer_sponsorships` · `promo.offer_promotions` ·
`promo.offer_views` · `promo.offer_clicks` · `promo.ads` · `promo.ad_plans` ·
`promo.ad_clicks` · `inventory.temp_warehouses` · `inventory.supplier_trackings` ·
`workflow.purchase_priority_engines` · `workflow.report_issues` ·
`org.branch_institutional_works` · `org.user_organization_numbers` ·
`hr.job_categories` · `hr.job_seeker_profiles` · `hr.work_times` ·
`identity.session_plans` (read-only) · `catalog.product_alerts` ·
`billing.payment_integrations` · `billing.payment_histories`

**21 tables — roughly 19% of the schema — are dead weight.**

---

## PART 7 — Workflows and business logic not migrated

| Workflow | Laravel mechanism | Status |
|---|---|---|
| Chunked spreadsheet import (thousands of rows, constant memory) | `ChunkReadFilter`, `ProcessMainImportChunk`, `ProcessImportJob`, `import_batches`/`import_rows`/`import_progress`, `/common/upload-chunk` | tables ported, **pipeline not** |
| AI drug-name matching | `ProductMatcher` 939 + `ArabicNormalizer` 127 + `FastSearchService` 364 | `aicapabilities` module exists; not wired to any mapping UI |
| Column auto-detection (Arabic + English headers) | `ColumnDetector` 814 | absent |
| Archive retention policy (8 customer / 22 vendor active files) | `compare_discount_plan_features` | absent |
| Offer rotation / sponsorship ordering | `OfferRotationEngine` 110 | `ORDER BY vo.is_sponsored DESC` only |
| Warehouse lifecycle | `WarehouseLifecycleService` 396 | absent |
| Geolocation supplier optimisation | `GeolocationSupplierOptimizer` 278 | absent |
| Order optimisation | `OrderOptimizationService` 305 + `AutomatedOrderOptimizationService` 370 | absent |
| Activity/error observability | `FullActivityLogService` 553, `FullErrorLogService` 692, 5 observers | `platform.audit_log` exists; observers absent |
| 2FA challenge | `Google2FAService` 124 | absent |
| Maintenance mode | `checkMaintenance` middleware | `platform_admin.feature_flags` partially covers |
| Multi-language content | `lang` middleware + `translations` + `/my-lang/{locale}` | ported ✓ |
| Full-power feature toggles | `full_powers` + `FullSettings` | ported as `feature_flags` ✓ |

---

## PART 8 — Inconsistencies to resolve before building further

1. **Migration 071's justification is factually wrong.** It states `saving_products`
   "held no data — its semantics are superseded by promo offers". `/what-in` lists Saving
   Products as a separate customer pillar *alongside* Offers, with its own admin, vendor
   and customer screens. The down migration cannot restore dropped rows. Decide
   explicitly: reinstate, or record the product decision to drop it.
2. **`billing.plans` is being used for compare-engine entitlements** via a `"compare"`
   string (`compare_handlers.go:53`). Laravel keeps a separate plan family with its own
   features, requests and seat/session tables. The shortcut cannot express the archive
   limits or the device cap.
3. **`commerce.orders.user_address_id` targets `identity.user_address_histories`**
   (migration 063) rather than `identity.user_addresses`. Laravel points
   `main_orders.user_address_id` at `user_addresses`. Ordering against a *history* row is
   unusual — confirm which is intended.
4. **`support` is seeded as staff with no reduced surface.** Either give the role a
   permission set and gate the pages, or stop seeding it.
5. **Swallowed errors are growing, not shrinking** (34 → 60). This needs a lint rule, not
   a cleanup pass.

---

## PART 9 — Prioritised backlog

Ordered by *what unblocks the most*, not by size.

### P0 — the platform cannot function without these

| # | Item | Why first | Done when |
|---|---|---|---|
| 1 | **Vendor coverage read + write** (§4.1) | every customer offer listing returns zero rows until a vendor can save coverage; nothing downstream is testable | vendor creates coverage in the UI; a customer in range sees the offer, one out of range does not — as a test |
| 2 | **Per-page admin permission gates** (§3.1) | `support` can currently run arbitrary SQL as system | each `/admin/*` route carries `RequirePermission`; `admin_guard_test` extended to UI routes; a support-role POST to `/admin/developers/sql` 404s |
| 3 | **Restrict the SQL console to `SELECT`/`EXPLAIN`; move `ensureDeveloperTables` into a migration** | defence in depth behind #2 | non-SELECT rejected, with a test |
| 4 | **Populate `coverages`; fix the 60 swallowed-error sites** | the bug class that makes working features look broken | lint rule in `.golangci.yml`; count at 0 in `internal/ui` |

### P1 — advertised features that do not exist

| # | Item | Notes |
|---|---|---|
| 5 | **Institutional-work filtering** on catalog, offer and supplier queries (§3.2) | needs `employee_institutional_works` first; implement Laravel's *Simple* mode on dashboard/catalog and *WithConnections* on purchase-request paths |
| 6 | **Discount-comparison engine** (§4.2) | 6 tables + upload + `ColumnDetector` + `ProductMatcher` + `ArabicNormalizer`. Largest single item — consider shipping *without* the AI matcher first; manual column mapping alone makes it usable |
| 7 | **Purchase Priority Engine** (§5.1) | table exists; port the 379-line service and one screen |
| 8 | **Automatic Purchase Request** | the 4,696-line service. Scope down to: upload → match → generate quote requests |
| 9 | **Chunked import pipeline** | prerequisite for #6 and #8 at real file sizes |
| 10 | **2FA enrollment + login challenge** (§4.3) | admin reset already exists |
| 11 | **Wire `JobApplySubmit`; link the 11 orphaned admin pages** | hours, not days |

### P2 — admin completeness

| # | Item |
|---|---|
| 12 | Role & permission editor (`/admin/roles`, `/admin/admin-roles`, `/admin/admin-permissions`) — currently **no way to edit a role in the product** |
| 13 | Trash / soft-delete recovery |
| 14 | Full user panel: customer-list / vendor-list / admin-list with create/edit/info/show |
| 15 | Error-log and activity-log pages (services already exist) |
| 16 | Contact-us inbox, AskFor, want-delete pages (POST handlers already exist) |
| 17 | Invoices / payments / wallets admin |
| 18 | Countries, currencies, brands, categories, stocks, warehouses admin |
| 19 | Branch products / branch users |
| 20 | System resources page |

### P3 — revenue surfaces (tables already built, zero UI)

| # | Item |
|---|---|
| 21 | Offer packages / sponsorships / promotions (vendor + admin) — 6 tables idle |
| 22 | Ads + ad plans (vendor + admin) — 3 tables idle |
| 23 | Offer views/clicks analytics |
| 24 | Session plans admin + request queue |
| 25 | Temp warehouses (12 admin routes + lifecycle service) |
| 26 | Saving Products — **after** the §8.1 decision |

### P4 — parity polish

Arabic PDF invoices · earnings reports · guest order tracking · supplier storefront
sub-pages · CPanel · vendor activities · job categories · vendor policies/social media ·
bulk employee upload · order/geo optimisation services · device-session tracking ·
subscription history.

---

## PART 10 — Standing rules for this work

Carried forward from AUDIT_V3 and reaffirmed by this pass:

1. **No fix ships without the check that would have caught it**, and that check must be
   shown *failing against the pre-fix tree*. `test/route_audience_test.go` was written to
   match the code rather than the plan, and stayed green through a completely dead admin
   panel.
2. **`go build` + `go vet` + `go test` are not evidence.** They were green while nobody
   could log in, migrations had never run, a core query could not bind, and the server
   panicked on boot. Go does not typecheck SQL, and nothing constructs the router.
3. **A dropped table needs a written product decision**, not a migration comment. See §8.1.
4. **A table with no route is a bug report**, not an asset. There are 21 today.
